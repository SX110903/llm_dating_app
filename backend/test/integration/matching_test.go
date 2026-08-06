package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

type matchingFixture struct {
	ID      uuid.UUID
	Token   string
	PhotoID uuid.UUID
}

type discoveryBody struct {
	Candidates []struct {
		UserID      string  `json:"user_id"`
		DisplayName string  `json:"display_name"`
		DistanceKM  float64 `json:"distance_km"`
		PhotoURL    string  `json:"photo_url"`
		Score       struct {
			Total float64 `json:"total"`
		} `json:"score"`
	} `json:"candidates"`
	NextCursor string `json:"next_cursor"`
}

// TestMatchingLifecycleRealInfrastructure exercises the complete Phase 3
// contract through HTTP while PostgreSQL/PostGIS and Redis are real:
// deterministic discovery, protected photos, concurrent mutual likes,
// unmatch authorization, bidirectional blocks, reports and the daily limit.
func TestMatchingLifecycleRealInfrastructure(t *testing.T) {
	pool := startPostgres(t)
	redisClient := startRedis(t)
	server := newTestServerWithMatchingLimit(t, pool, redisClient, false, []string{"http://localhost:5173"}, 2)

	alice := createMatchingFixture(t, pool, server, "Alice", "woman", 40.4168, -3.7038,
		[]string{"man"}, []string{"hiking", "music"}, map[string]any{"weekend": "mountains"})
	bob := createMatchingFixture(t, pool, server, "Bob", "man", 40.4168, -3.7038,
		[]string{"woman"}, []string{"hiking", "music"}, map[string]any{"weekend": "mountains"})
	dave := createMatchingFixture(t, pool, server, "Dave", "man", 40.4300, -3.7000,
		[]string{"woman"}, []string{"cooking"}, map[string]any{"weekend": "city"})
	far := createMatchingFixture(t, pool, server, "Far", "man", 41.3874, 2.1686,
		[]string{"woman"}, []string{"hiking"}, map[string]any{"weekend": "mountains"})
	eve := createMatchingFixture(t, pool, server, "Eve", "man", 40.4400, -3.7100,
		[]string{"woman"}, []string{"books"}, map[string]any{"weekend": "city"})
	frank := createMatchingFixture(t, pool, server, "Frank", "man", 40.4500, -3.7200,
		[]string{"woman"}, []string{"cinema"}, map[string]any{"weekend": "city"})

	firstPage := getDiscovery(t, server, alice.Token, "?limit=2")
	require.Len(t, firstPage.Candidates, 2)
	require.Equal(t, bob.ID.String(), firstPage.Candidates[0].UserID,
		"the exact interest/questionnaire fixture must rank first")
	require.NotEmpty(t, firstPage.NextCursor)
	require.Greater(t, firstPage.Candidates[0].Score.Total, firstPage.Candidates[1].Score.Total)

	secondPage := getDiscovery(t, server, alice.Token, "?limit=2&cursor="+firstPage.NextCursor)
	firstIDs := candidateIDs(firstPage)
	for _, candidate := range secondPage.Candidates {
		require.NotContains(t, firstIDs, candidate.UserID, "cursor pages must not overlap")
	}
	allVisible := append(firstPage.Candidates, secondPage.Candidates...)
	require.NotContains(t, candidateIDs(discoveryBody{Candidates: allVisible}), far.ID.String(),
		"the Barcelona fixture must fail the PostGIS distance filter")

	photoResponse := authedJSONRequest(t, server, http.MethodGet, firstPage.Candidates[0].PhotoURL, alice.Token, nil)
	defer func() { _ = photoResponse.Body.Close() }()
	require.Equal(t, http.StatusOK, photoResponse.StatusCode)
	require.Equal(t, "image/png", photoResponse.Header.Get("Content-Type"))

	farPhoto := authedJSONRequest(t, server, http.MethodGet,
		"/api/v1/matching/photos/"+far.PhotoID.String()+"/content", alice.Token, nil)
	defer func() { _ = farPhoto.Body.Close() }()
	require.Equal(t, http.StatusNotFound, farPhoto.StatusCode,
		"an ineligible candidate photo must stay private")

	blockResponse := authedJSONRequest(t, server, http.MethodPost, "/api/v1/blocks", alice.Token,
		map[string]string{"user_id": dave.ID.String()})
	defer func() { _ = blockResponse.Body.Close() }()
	require.Equal(t, http.StatusNoContent, blockResponse.StatusCode)
	require.NotContains(t, candidateIDs(getDiscovery(t, server, alice.Token, "?limit=50")), dave.ID.String())
	require.NotContains(t, candidateIDs(getDiscovery(t, server, dave.Token, "?limit=50")), alice.ID.String(),
		"a block must hide both directions")

	statuses, outcomes := concurrentLikes(t, server, alice, bob)
	require.ElementsMatch(t, []int{http.StatusCreated, http.StatusCreated}, statuses)
	matchCount := 0
	matchID := ""
	for _, outcome := range outcomes {
		if outcome.Match != nil {
			matchCount++
			matchID = outcome.Match.ID
		}
	}
	require.Equal(t, 1, matchCount, "only the transaction that observes the reverse like creates the match")
	require.NotEmpty(t, matchID)

	var persistedMatches int
	var ordered bool
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*), bool_and(user_low_id < user_high_id) FROM matches`,
	).Scan(&persistedMatches, &ordered))
	require.Equal(t, 1, persistedMatches)
	require.True(t, ordered)

	listAlice := authedJSONRequest(t, server, http.MethodGet, "/api/v1/matches", alice.Token, nil)
	defer func() { _ = listAlice.Body.Close() }()
	require.Equal(t, http.StatusOK, listAlice.StatusCode)
	var matches struct {
		Matches []struct {
			ID          string `json:"id"`
			OtherUserID string `json:"other_user_id"`
		} `json:"matches"`
	}
	require.NoError(t, json.NewDecoder(listAlice.Body).Decode(&matches))
	require.Len(t, matches.Matches, 1)
	require.Equal(t, bob.ID.String(), matches.Matches[0].OtherUserID)

	unauthorized := authedJSONRequest(t, server, http.MethodDelete, "/api/v1/matches/"+matchID, far.Token, nil)
	defer func() { _ = unauthorized.Body.Close() }()
	require.Equal(t, http.StatusNotFound, unauthorized.StatusCode)

	unmatched := authedJSONRequest(t, server, http.MethodDelete, "/api/v1/matches/"+matchID, alice.Token, nil)
	defer func() { _ = unmatched.Body.Close() }()
	require.Equal(t, http.StatusNoContent, unmatched.StatusCode)
	repeated := authedJSONRequest(t, server, http.MethodDelete, "/api/v1/matches/"+matchID, alice.Token, nil)
	defer func() { _ = repeated.Body.Close() }()
	require.Equal(t, http.StatusNoContent, repeated.StatusCode, "unmatch must be idempotent for a participant")

	secondSwipe := authedJSONRequest(t, server, http.MethodPost, "/api/v1/swipes", alice.Token,
		map[string]string{"target_id": eve.ID.String(), "action": "dislike"})
	defer func() { _ = secondSwipe.Body.Close() }()
	require.Equal(t, http.StatusCreated, secondSwipe.StatusCode)
	limited := authedJSONRequest(t, server, http.MethodPost, "/api/v1/swipes", alice.Token,
		map[string]string{"target_id": frank.ID.String(), "action": "like"})
	defer func() { _ = limited.Body.Close() }()
	require.Equal(t, http.StatusTooManyRequests, limited.StatusCode)
	require.NotEmpty(t, limited.Header.Get("Retry-After"))

	reportResponse := authedJSONRequest(t, server, http.MethodPost, "/api/v1/reports", alice.Token, map[string]string{
		"user_id": frank.ID.String(), "reason": "spam", "description": "<script>bad()</script><b>context</b>",
	})
	defer func() { _ = reportResponse.Body.Close() }()
	require.Equal(t, http.StatusCreated, reportResponse.StatusCode)
	var report struct {
		Description string `json:"description"`
	}
	require.NoError(t, json.NewDecoder(reportResponse.Body).Decode(&report))
	require.NotContains(t, report.Description, "<")
	require.NotContains(t, report.Description, "bad")
}

func createMatchingFixture(
	t *testing.T,
	pool *pgxpool.Pool,
	server *httptest.Server,
	displayName, gender string,
	latitude, longitude float64,
	genders, interests []string,
	questionnaire map[string]any,
) matchingFixture {
	t.Helper()
	email := uniqueEmail()
	registerUserWithDetails(t, server, email, testPassword, displayName, gender)
	token := loginAccessToken(t, server, email, testPassword)

	profilePayload := map[string]any{
		"bio": displayName + " fixture", "city": displayName + " city", "interests": interests,
		"questionnaire": questionnaire, "latitude": latitude, "longitude": longitude,
		"onboarding_completed": false,
	}
	profile := authedJSONRequest(t, server, http.MethodPut, "/api/v1/profile", token, profilePayload)
	defer func() { _ = profile.Body.Close() }()
	require.Equal(t, http.StatusOK, profile.StatusCode)

	consent := authedJSONRequest(t, server, http.MethodPost, "/api/v1/account/consents", token,
		map[string]string{"purpose": "matching_gender_preferences"})
	defer func() { _ = consent.Body.Close() }()
	require.Equal(t, http.StatusCreated, consent.StatusCode)
	preferences := authedJSONRequest(t, server, http.MethodPut, "/api/v1/profile/preferences", token, map[string]any{
		"min_age": 18, "max_age": 60, "max_distance_km": 80, "genders": genders,
	})
	defer func() { _ = preferences.Body.Close() }()
	require.Equal(t, http.StatusOK, preferences.StatusCode)

	uploaded := uploadPhoto(t, server, token, strings.ToLower(displayName)+".png", encodeTestPNG(t))
	defer func() { _ = uploaded.Body.Close() }()
	require.Equal(t, http.StatusCreated, uploaded.StatusCode)
	var photo struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.NewDecoder(uploaded.Body).Decode(&photo))

	profilePayload["onboarding_completed"] = true
	delete(profilePayload, "latitude")
	delete(profilePayload, "longitude")
	completed := authedJSONRequest(t, server, http.MethodPut, "/api/v1/profile", token, profilePayload)
	defer func() { _ = completed.Body.Close() }()
	require.Equal(t, http.StatusOK, completed.StatusCode)

	var userID uuid.UUID
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT id FROM users WHERE email = $1`, email).Scan(&userID))
	return matchingFixture{ID: userID, Token: token, PhotoID: uuid.MustParse(photo.ID)}
}

func getDiscovery(t *testing.T, server *httptest.Server, token, query string) discoveryBody {
	t.Helper()
	response := authedJSONRequest(t, server, http.MethodGet, "/api/v1/discovery"+query, token, nil)
	defer func() { _ = response.Body.Close() }()
	require.Equal(t, http.StatusOK, response.StatusCode)
	var body discoveryBody
	require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
	return body
}

func candidateIDs(discovery discoveryBody) []string {
	ids := make([]string, 0, len(discovery.Candidates))
	for _, candidate := range discovery.Candidates {
		ids = append(ids, candidate.UserID)
	}
	return ids
}

type concurrentSwipeResult struct {
	Match *struct {
		ID string `json:"id"`
	} `json:"match"`
}

func concurrentLikes(
	t *testing.T,
	server *httptest.Server,
	first, second matchingFixture,
) ([]int, []concurrentSwipeResult) {
	t.Helper()
	type requestInput struct {
		actor  matchingFixture
		target matchingFixture
	}
	inputs := []requestInput{{actor: first, target: second}, {actor: second, target: first}}
	statuses := make([]int, len(inputs))
	outcomes := make([]concurrentSwipeResult, len(inputs))
	errorsFound := make([]error, len(inputs))

	var waitGroup sync.WaitGroup
	for index, input := range inputs {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			payload, err := json.Marshal(map[string]string{"target_id": input.target.ID.String(), "action": "like"})
			if err != nil {
				errorsFound[index] = err
				return
			}
			request, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
				server.URL+"/api/v1/swipes", bytes.NewReader(payload))
			if err != nil {
				errorsFound[index] = err
				return
			}
			request.Header.Set("Authorization", "Bearer "+input.actor.Token)
			request.Header.Set("Content-Type", "application/json")
			response, err := server.Client().Do(request)
			if err != nil {
				errorsFound[index] = err
				return
			}
			statuses[index] = response.StatusCode
			raw, readErr := io.ReadAll(response.Body)
			_ = response.Body.Close()
			if readErr != nil {
				errorsFound[index] = readErr
				return
			}
			if decodeErr := json.Unmarshal(raw, &outcomes[index]); decodeErr != nil {
				errorsFound[index] = decodeErr
			}
		}()
	}
	waitGroup.Wait()
	for _, err := range errorsFound {
		require.NoError(t, err)
	}
	return statuses, outcomes
}
