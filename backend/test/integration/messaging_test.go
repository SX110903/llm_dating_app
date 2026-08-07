package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	redisadapter "github.com/sx110903/llmatch-v2/backend/internal/adapters/redis"
	applicationmessaging "github.com/sx110903/llmatch-v2/backend/internal/application/messaging"
)

type chatFixture struct {
	server  *httptest.Server
	matchID string
	aliceTk string
	bobTk   string
	aliceID string
	bobID   string
}

// newChatFixture drives the real HTTP surface end to end -- register, profile,
// consent, photo, mutual like -- so the conversation under test sits on top of
// genuinely matched users rather than hand-seeded rows.
func newChatFixture(t *testing.T, server *httptest.Server) *chatFixture {
	t.Helper()

	aliceEmail, bobEmail := uniqueEmail(), uniqueEmail()
	registerUserWithDetails(t, server, aliceEmail, testPassword, "Ana", "woman")
	registerUserWithDetails(t, server, bobEmail, testPassword, "Beto", "man")
	aliceToken := loginAccessToken(t, server, aliceEmail, testPassword)
	bobToken := loginAccessToken(t, server, bobEmail, testPassword)

	prepareDiscoverableProfile(t, server, aliceToken)
	prepareDiscoverableProfile(t, server, bobToken)

	aliceID := currentUserID(t, server, aliceToken)
	bobID := currentUserID(t, server, bobToken)

	swipe(t, server, aliceToken, bobID)
	matchID := swipeExpectingMatch(t, server, bobToken, aliceID)

	return &chatFixture{
		server: server, matchID: matchID,
		aliceTk: aliceToken, bobTk: bobToken,
		aliceID: aliceID, bobID: bobID,
	}
}

func prepareDiscoverableProfile(t *testing.T, server *httptest.Server, token string) {
	t.Helper()
	resp := authedJSONRequest(t, server, http.MethodPut, "/api/v1/profile", token, map[string]any{
		"bio": "hola", "city": "Madrid", "interests": []string{"cine"},
		"latitude": 40.4168, "longitude": -3.7038, "onboarding_completed": false,
	})
	_ = resp.Body.Close()

	consent := authedJSONRequest(t, server, http.MethodPost, "/api/v1/account/consents", token, map[string]any{
		"purpose": "matching_gender_preferences",
	})
	_ = consent.Body.Close()

	prefs := authedJSONRequest(t, server, http.MethodPut, "/api/v1/profile/preferences", token, map[string]any{
		"min_age": 18, "max_age": 99, "max_distance_km": 500, "genders": []string{"woman", "man"},
	})
	_ = prefs.Body.Close()

	photo := uploadPhoto(t, server, token, "p.png", encodeTestPNG(t))
	_ = photo.Body.Close()

	done := authedJSONRequest(t, server, http.MethodPut, "/api/v1/profile", token, map[string]any{
		"bio": "hola", "city": "Madrid", "interests": []string{"cine"}, "onboarding_completed": true,
	})
	_ = done.Body.Close()
}

func currentUserID(t *testing.T, server *httptest.Server, token string) string {
	t.Helper()
	resp := authedJSONRequest(t, server, http.MethodGet, "/api/v1/auth/me", token, nil)
	defer func() { _ = resp.Body.Close() }()
	var body struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	return body.ID
}

func swipe(t *testing.T, server *httptest.Server, token, targetID string) {
	t.Helper()
	resp := authedJSONRequest(t, server, http.MethodPost, "/api/v1/swipes", token, map[string]any{
		"target_id": targetID, "action": "like",
	})
	_ = resp.Body.Close()
}

func swipeExpectingMatch(t *testing.T, server *httptest.Server, token, targetID string) string {
	t.Helper()
	resp := authedJSONRequest(t, server, http.MethodPost, "/api/v1/swipes", token, map[string]any{
		"target_id": targetID, "action": "like",
	})
	defer func() { _ = resp.Body.Close() }()
	var body struct {
		Match *struct {
			ID string `json:"id"`
		} `json:"match"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.NotNil(t, body.Match, "the mutual like should have produced a match")
	return body.Match.ID
}

func sendMessage(t *testing.T, server *httptest.Server, token, matchID, nonce, content string) *http.Response {
	t.Helper()
	return authedJSONRequest(t, server, http.MethodPost, "/api/v1/matches/"+matchID+"/messages", token, map[string]any{
		"client_nonce": nonce, "type": "text", "content": content,
	})
}

// --- 1. idempotency against the real unique index --------------------------

func TestSendingTheSameNonceTwiceStoresOneMessage(t *testing.T) {
	pool := startPostgres(t)
	redisClient := startRedis(t)
	server := newTestServer(t, pool, redisClient, false, []string{"http://localhost:5173"})
	chat := newChatFixture(t, server)

	nonce := uuid.NewString()

	first := sendMessage(t, server, chat.aliceTk, chat.matchID, nonce, "hola")
	defer func() { _ = first.Body.Close() }()
	require.Equal(t, http.StatusCreated, first.StatusCode)
	var created struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.NewDecoder(first.Body).Decode(&created))

	second := sendMessage(t, server, chat.aliceTk, chat.matchID, nonce, "hola")
	defer func() { _ = second.Body.Close() }()
	require.Equal(t, http.StatusOK, second.StatusCode, "a replay must not create a new message")
	var replayed struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.NewDecoder(second.Body).Decode(&replayed))
	require.Equal(t, created.ID, replayed.ID)

	var stored int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM messages WHERE match_id = $1`, chat.matchID).Scan(&stored))
	require.Equal(t, 1, stored)
}

// --- 2. concurrent sends ---------------------------------------------------

func TestConcurrentSendsWithTheSameNonceStoreOneRow(t *testing.T) {
	pool := startPostgres(t)
	redisClient := startRedis(t)
	server := newTestServer(t, pool, redisClient, false, []string{"http://localhost:5173"})
	chat := newChatFixture(t, server)

	nonce := uuid.NewString()
	const attempts = 8
	statuses := make([]int, attempts)
	var wg sync.WaitGroup
	for i := range attempts {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			resp := sendMessage(t, server, chat.aliceTk, chat.matchID, nonce, "carrera")
			defer func() { _ = resp.Body.Close() }()
			statuses[index] = resp.StatusCode
		}(i)
	}
	wg.Wait()

	created := 0
	for _, status := range statuses {
		if status == http.StatusCreated {
			created++
		}
	}
	require.Equal(t, 1, created, "exactly one concurrent send may create the message")

	var stored int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM messages WHERE match_id = $1`, chat.matchID).Scan(&stored))
	require.Equal(t, 1, stored)
}

// --- 3. outsiders ----------------------------------------------------------

func TestOutsiderCannotReadOrWriteSomebodyElsesConversation(t *testing.T) {
	pool := startPostgres(t)
	redisClient := startRedis(t)
	server := newTestServer(t, pool, redisClient, false, []string{"http://localhost:5173"})
	chat := newChatFixture(t, server)

	intruderEmail := uniqueEmail()
	registerUser(t, server, intruderEmail, testPassword)
	intruderToken := loginAccessToken(t, server, intruderEmail, testPassword)

	write := sendMessage(t, server, intruderToken, chat.matchID, uuid.NewString(), "déjame entrar")
	defer func() { _ = write.Body.Close() }()
	require.Equal(t, http.StatusNotFound, write.StatusCode)

	read := authedJSONRequest(t, server, http.MethodGet, "/api/v1/matches/"+chat.matchID+"/messages", intruderToken, nil)
	defer func() { _ = read.Body.Close() }()
	require.Equal(t, http.StatusNotFound, read.StatusCode)

	markRead := authedJSONRequest(t, server, http.MethodPost, "/api/v1/matches/"+chat.matchID+"/messages/read", intruderToken, nil)
	defer func() { _ = markRead.Body.Close() }()
	require.Equal(t, http.StatusNotFound, markRead.StatusCode)
}

// --- 4. unmatch and block --------------------------------------------------

func TestUnmatchStopsNewMessages(t *testing.T) {
	pool := startPostgres(t)
	redisClient := startRedis(t)
	server := newTestServer(t, pool, redisClient, false, []string{"http://localhost:5173"})
	chat := newChatFixture(t, server)

	before := sendMessage(t, server, chat.aliceTk, chat.matchID, uuid.NewString(), "antes")
	_ = before.Body.Close()
	require.Equal(t, http.StatusCreated, before.StatusCode)

	unmatch := authedJSONRequest(t, server, http.MethodDelete, "/api/v1/matches/"+chat.matchID, chat.aliceTk, nil)
	_ = unmatch.Body.Close()
	require.Equal(t, http.StatusNoContent, unmatch.StatusCode)

	after := sendMessage(t, server, chat.aliceTk, chat.matchID, uuid.NewString(), "después")
	defer func() { _ = after.Body.Close() }()
	require.Equal(t, http.StatusNotFound, after.StatusCode, "an unmatched conversation must reject new messages")
}

func TestBlockStopsMessagesInBothDirections(t *testing.T) {
	pool := startPostgres(t)
	redisClient := startRedis(t)
	server := newTestServer(t, pool, redisClient, false, []string{"http://localhost:5173"})
	chat := newChatFixture(t, server)

	block := authedJSONRequest(t, server, http.MethodPost, "/api/v1/blocks", chat.aliceTk, map[string]any{
		"user_id": chat.bobID,
	})
	_ = block.Body.Close()
	// Asserted so a rejected block cannot make the rest of the test pass for
	// the wrong reason.
	require.Equal(t, http.StatusNoContent, block.StatusCode)

	fromBlocker := sendMessage(t, server, chat.aliceTk, chat.matchID, uuid.NewString(), "desde quien bloquea")
	defer func() { _ = fromBlocker.Body.Close() }()
	require.Equal(t, http.StatusNotFound, fromBlocker.StatusCode)

	fromBlocked := sendMessage(t, server, chat.bobTk, chat.matchID, uuid.NewString(), "desde quien fue bloqueado")
	defer func() { _ = fromBlocked.Body.Close() }()
	require.Equal(t, http.StatusNotFound, fromBlocked.StatusCode,
		"a block must cut the conversation in both directions")
}

// --- 5. cursor over real SQL -----------------------------------------------

func TestHistoryCursorHasNoGapsOrDuplicates(t *testing.T) {
	pool := startPostgres(t)
	redisClient := startRedis(t)
	server := newTestServer(t, pool, redisClient, false, []string{"http://localhost:5173"})
	chat := newChatFixture(t, server)

	const total = 17
	for i := range total {
		resp := sendMessage(t, server, chat.aliceTk, chat.matchID, uuid.NewString(), "mensaje")
		_ = resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		_ = i
	}

	seen := map[string]bool{}
	cursor := ""
	for pages := 0; ; pages++ {
		require.Less(t, pages, 10, "pagination did not terminate")
		path := "/api/v1/matches/" + chat.matchID + "/messages?limit=5"
		if cursor != "" {
			path += "&cursor=" + cursor
		}
		resp := authedJSONRequest(t, server, http.MethodGet, path, chat.aliceTk, nil)
		var page struct {
			Messages []struct {
				ID string `json:"id"`
			} `json:"messages"`
			NextCursor string `json:"next_cursor"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&page))
		_ = resp.Body.Close()

		for _, message := range page.Messages {
			require.False(t, seen[message.ID], "the cursor returned a duplicate")
			seen[message.ID] = true
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	require.Len(t, seen, total, "the cursor skipped messages")
}

// --- 6. ticket lifecycle over real Redis -----------------------------------

func TestWebSocketTicketIsSingleUseAndExpires(t *testing.T) {
	pool := startPostgres(t)
	redisClient := startRedis(t)
	server := newTestServer(t, pool, redisClient, false, []string{"http://localhost:5173"})
	chat := newChatFixture(t, server)

	resp := authedJSONRequest(t, server, http.MethodPost, "/api/v1/messaging/tickets", chat.aliceTk, nil)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var issued struct {
		Ticket string `json:"ticket"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&issued))
	require.NotEmpty(t, issued.Ticket)

	store := redisadapter.NewWSTicketStore(redisClient)

	owner, err := store.Consume(context.Background(), issued.Ticket)
	require.NoError(t, err)
	require.Equal(t, chat.aliceID, owner.String())

	_, err = store.Consume(context.Background(), issued.Ticket)
	require.Error(t, err, "a ticket must not be redeemable twice")

	_, err = store.Consume(context.Background(), issued.Ticket+"tampered")
	require.Error(t, err)

	// An expired ticket is indistinguishable from an unknown one.
	require.NoError(t, store.Issue(context.Background(), "short-lived", uuid.New(), 50*time.Millisecond))
	time.Sleep(200 * time.Millisecond)
	_, err = store.Consume(context.Background(), "short-lived")
	require.Error(t, err)
}

// --- 8. cross-instance fan-out ---------------------------------------------

// TestMessageReachesAnotherInstanceThroughRedis proves the Pub/Sub path: the
// message is sent to one server while a second server's websocket handler is
// the only subscriber, and the event still arrives.
func TestMessageReachesAnotherInstanceThroughRedis(t *testing.T) {
	pool := startPostgres(t)
	redisClient := startRedis(t)

	sender, _ := newTestServerWithMessaging(t, pool, redisClient, false, []string{"http://localhost:5173"}, 100)
	_, secondInstance := newTestServerWithMessaging(t, pool, redisClient, false, []string{"http://localhost:5173"}, 100)
	chat := newChatFixture(t, sender)

	received := make(chan applicationmessaging.MessageEvent, 4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		bus := redisadapter.NewMessageBus(redisClient)
		_ = bus.Subscribe(ctx, func(event applicationmessaging.MessageEvent) {
			secondInstance.Broadcast(event)
			received <- event
		})
	}()
	// Give the subscription time to attach before publishing.
	time.Sleep(500 * time.Millisecond)

	resp := sendMessage(t, sender, chat.aliceTk, chat.matchID, uuid.NewString(), "cruza instancias")
	_ = resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	select {
	case event := <-received:
		require.Equal(t, chat.matchID, event.MatchID.String())
		require.Equal(t, "cruza instancias", event.Message.Content)
		require.Len(t, event.Recipients, 2)
	case <-time.After(5 * time.Second):
		t.Fatal("the message never reached the second instance through Redis")
	}
}

// --- 11. read receipts over real SQL ---------------------------------------

func TestMarkReadIsIdempotentAndOnlyAffectsReceivedMessages(t *testing.T) {
	pool := startPostgres(t)
	redisClient := startRedis(t)
	server := newTestServer(t, pool, redisClient, false, []string{"http://localhost:5173"})
	chat := newChatFixture(t, server)

	sent := sendMessage(t, server, chat.aliceTk, chat.matchID, uuid.NewString(), "para bob")
	_ = sent.Body.Close()

	first := authedJSONRequest(t, server, http.MethodPost, "/api/v1/matches/"+chat.matchID+"/messages/read", chat.bobTk, nil)
	var firstBody struct {
		Updated int `json:"updated"`
	}
	require.NoError(t, json.NewDecoder(first.Body).Decode(&firstBody))
	_ = first.Body.Close()
	require.Equal(t, 1, firstBody.Updated)

	second := authedJSONRequest(t, server, http.MethodPost, "/api/v1/matches/"+chat.matchID+"/messages/read", chat.bobTk, nil)
	var secondBody struct {
		Updated int `json:"updated"`
	}
	require.NoError(t, json.NewDecoder(second.Body).Decode(&secondBody))
	_ = second.Body.Close()
	require.Equal(t, 0, secondBody.Updated, "marking read twice must change nothing")

	// The sender marking their own conversation must not flag their own message.
	own := authedJSONRequest(t, server, http.MethodPost, "/api/v1/matches/"+chat.matchID+"/messages/read", chat.aliceTk, nil)
	var ownBody struct {
		Updated int `json:"updated"`
	}
	require.NoError(t, json.NewDecoder(own.Body).Decode(&ownBody))
	_ = own.Body.Close()
	require.Equal(t, 0, ownBody.Updated)
}

// --- security: no client-supplied storage keys -----------------------------

// TestClientCannotAttachAnArbitraryStorageKey guards the IDOR that would let a
// sender point a message at an object owned by somebody else.
func TestClientCannotAttachAnArbitraryStorageKey(t *testing.T) {
	pool := startPostgres(t)
	redisClient := startRedis(t)
	server := newTestServer(t, pool, redisClient, false, []string{"http://localhost:5173"})
	chat := newChatFixture(t, server)

	resp := authedJSONRequest(t, server, http.MethodPost, "/api/v1/matches/"+chat.matchID+"/messages", chat.aliceTk, map[string]any{
		"client_nonce": uuid.NewString(),
		"type":         "image",
		"storage_key":  "photos/somebody-else/stolen",
	})
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"an unknown storage_key field must be rejected outright")

	var stored int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM messages WHERE storage_key IS NOT NULL`).Scan(&stored))
	require.Equal(t, 0, stored, "no message may reference a client-supplied object key")
}

// --- conversation list -----------------------------------------------------

func TestConversationListShowsUnreadCountAndLastMessage(t *testing.T) {
	pool := startPostgres(t)
	redisClient := startRedis(t)
	server := newTestServer(t, pool, redisClient, false, []string{"http://localhost:5173"})
	chat := newChatFixture(t, server)

	for _, text := range []string{"uno", "dos"} {
		resp := sendMessage(t, server, chat.aliceTk, chat.matchID, uuid.NewString(), text)
		_ = resp.Body.Close()
	}

	resp := authedJSONRequest(t, server, http.MethodGet, "/api/v1/conversations", chat.bobTk, nil)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body struct {
		Conversations []struct {
			MatchID     string `json:"match_id"`
			UnreadCount int    `json:"unread_count"`
			LastMessage *struct {
				Content string `json:"content"`
			} `json:"last_message"`
		} `json:"conversations"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Len(t, body.Conversations, 1)
	require.Equal(t, chat.matchID, body.Conversations[0].MatchID)
	require.Equal(t, 2, body.Conversations[0].UnreadCount)
	require.NotNil(t, body.Conversations[0].LastMessage)
	require.Equal(t, "dos", body.Conversations[0].LastMessage.Content)
}
