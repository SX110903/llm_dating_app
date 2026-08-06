package messaging_test

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	applicationmessaging "github.com/sx110903/llmatch-v2/backend/internal/application/messaging"
	domainmessaging "github.com/sx110903/llmatch-v2/backend/internal/domain/messaging"
)

// --- doubles ---------------------------------------------------------------

// fakeRepo models the parts of the contract that matter to the service: the
// (sender_id, client_nonce) uniqueness that makes sends idempotent, and the
// authorization gate that hides inaccessible matches.
type fakeRepo struct {
	mu           sync.Mutex
	participants map[uuid.UUID]*domainmessaging.Participants
	byNonce      map[string]domainmessaging.Message
	messages     []domainmessaging.Message
	sendErr      error
	sendCalls    int
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		participants: map[uuid.UUID]*domainmessaging.Participants{},
		byNonce:      map[string]domainmessaging.Message{},
	}
}

func (r *fakeRepo) allow(matchID, low, high uuid.UUID) {
	r.participants[matchID] = &domainmessaging.Participants{
		MatchID: matchID, UserLowID: low, UserHighID: high, MatchedAt: time.Now().UTC(),
	}
}

func (r *fakeRepo) GetActiveParticipants(_ context.Context, matchID, viewerID uuid.UUID) (*domainmessaging.Participants, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.participants[matchID]
	if !ok || (p.UserLowID != viewerID && p.UserHighID != viewerID) {
		return nil, domainmessaging.ErrMatchNotAccessible
	}
	copied := *p
	return &copied, nil
}

func nonceKey(senderID, nonce uuid.UUID) string { return senderID.String() + "|" + nonce.String() }

func (r *fakeRepo) Send(_ context.Context, message *domainmessaging.Message) (*domainmessaging.SendResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sendCalls++
	if r.sendErr != nil {
		return nil, r.sendErr
	}
	key := nonceKey(message.SenderID, message.ClientNonce)
	if existing, ok := r.byNonce[key]; ok {
		return &domainmessaging.SendResult{Message: existing, Created: false}, nil
	}
	stored := *message
	stored.CreatedAt = time.Now().UTC()
	r.byNonce[key] = stored
	r.messages = append(r.messages, stored)
	return &domainmessaging.SendResult{Message: stored, Created: true}, nil
}

func (r *fakeRepo) ListHistory(_ context.Context, params domainmessaging.HistoryParams) ([]domainmessaging.Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	page := make([]domainmessaging.Message, 0, params.Limit)
	ordered := append([]domainmessaging.Message(nil), r.messages...)
	sort.Slice(ordered, func(i, j int) bool {
		if !ordered[i].CreatedAt.Equal(ordered[j].CreatedAt) {
			return ordered[i].CreatedAt.After(ordered[j].CreatedAt)
		}
		return ordered[i].ID.String() > ordered[j].ID.String()
	})
	for _, m := range ordered {
		if m.MatchID != params.MatchID {
			continue
		}
		if params.Before != nil {
			after := m.CreatedAt.After(params.Before.CreatedAt)
			same := m.CreatedAt.Equal(params.Before.CreatedAt) && m.ID.String() >= params.Before.MessageID.String()
			if after || same {
				continue
			}
		}
		page = append(page, m)
		if len(page) == params.Limit {
			break
		}
	}
	return page, nil
}

func (r *fakeRepo) MarkRead(_ context.Context, matchID, viewerID uuid.UUID, at time.Time) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	updated := 0
	for i := range r.messages {
		m := &r.messages[i]
		if m.MatchID == matchID && m.SenderID != viewerID && m.ReadAt == nil {
			readAt := at
			m.ReadAt = &readAt
			updated++
		}
	}
	return updated, nil
}

func (r *fakeRepo) ListConversations(context.Context, uuid.UUID, int) ([]domainmessaging.ConversationSummary, error) {
	return nil, nil
}

type fakeTickets struct {
	mu       sync.Mutex
	stored   map[string]uuid.UUID
	issueErr error
	failHard error
}

func newFakeTickets() *fakeTickets { return &fakeTickets{stored: map[string]uuid.UUID{}} }

func (t *fakeTickets) Issue(_ context.Context, ticket string, userID uuid.UUID, _ time.Duration) error {
	if t.issueErr != nil {
		return t.issueErr
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.stored[ticket] = userID
	return nil
}

func (t *fakeTickets) Consume(_ context.Context, ticket string) (uuid.UUID, error) {
	if t.failHard != nil {
		return uuid.Nil, t.failHard
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	userID, ok := t.stored[ticket]
	if !ok {
		return uuid.Nil, domainmessaging.ErrTicketInvalid
	}
	delete(t.stored, ticket)
	return userID, nil
}

type recordingPublisher struct {
	mu        sync.Mutex
	published []applicationmessaging.MessageEvent
	err       error
	// onPublish observes repository state at publish time, which is how the
	// "persist before publish" ordering is proven.
	onPublish func()
}

func (p *recordingPublisher) Publish(_ context.Context, event applicationmessaging.MessageEvent) error {
	if p.onPublish != nil {
		p.onPublish()
	}
	if p.err != nil {
		return p.err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.published = append(p.published, event)
	return nil
}

func (p *recordingPublisher) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.published)
}

type fakeLimiter struct {
	allowed    bool
	retryAfter time.Duration
	err        error
}

func (l fakeLimiter) Allow(context.Context, uuid.UUID) (bool, time.Duration, error) {
	if l.err != nil {
		return false, 0, l.err
	}
	return l.allowed, l.retryAfter, nil
}

type harness struct {
	service   *applicationmessaging.Service
	repo      *fakeRepo
	tickets   *fakeTickets
	publisher *recordingPublisher
	matchID   uuid.UUID
	alice     uuid.UUID
	bob       uuid.UUID
}

func newHarness(t *testing.T, limiter applicationmessaging.RateLimiter) *harness {
	t.Helper()
	repo := newFakeRepo()
	tickets := newFakeTickets()
	publisher := &recordingPublisher{}
	if limiter == nil {
		limiter = fakeLimiter{allowed: true}
	}
	h := &harness{
		repo: repo, tickets: tickets, publisher: publisher,
		matchID: uuid.New(), alice: uuid.New(), bob: uuid.New(),
	}
	low, high := h.alice, h.bob
	if low.String() > high.String() {
		low, high = high, low
	}
	repo.allow(h.matchID, low, high)
	h.service = applicationmessaging.NewService(repo, tickets, publisher, limiter,
		applicationmessaging.Config{TicketTTL: 30 * time.Second})
	return h
}

func (h *harness) send(t *testing.T, sender uuid.UUID, nonce uuid.UUID, content string) (*domainmessaging.SendResult, error) {
	t.Helper()
	return h.service.SendText(context.Background(), applicationmessaging.SendTextInput{
		MatchID: h.matchID, SenderID: sender, ClientNonce: nonce, Content: content,
	})
}

// --- 1. idempotent send ----------------------------------------------------

func TestSendWithRepeatedNonceStoresOneMessage(t *testing.T) {
	h := newHarness(t, nil)
	nonce := uuid.New()

	first, err := h.send(t, h.alice, nonce, "hola")
	require.NoError(t, err)
	require.True(t, first.Created)

	second, err := h.send(t, h.alice, nonce, "hola")
	require.NoError(t, err)
	require.False(t, second.Created, "a replayed nonce must not create a second message")
	require.Equal(t, first.Message.ID, second.Message.ID)
	require.Len(t, h.repo.messages, 1)
	require.Equal(t, 1, h.publisher.count(), "a replay must not be broadcast again")
}

// --- 2. concurrent sends ---------------------------------------------------

func TestConcurrentSendsWithSameNonceCreateOneMessage(t *testing.T) {
	h := newHarness(t, nil)
	nonce := uuid.New()

	const attempts = 12
	results := make([]*domainmessaging.SendResult, attempts)
	var wg sync.WaitGroup
	for i := range attempts {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			result, err := h.send(t, h.alice, nonce, "carrera")
			if err == nil {
				results[index] = result
			}
		}(i)
	}
	wg.Wait()

	created := 0
	for _, result := range results {
		if result != nil && result.Created {
			created++
		}
	}
	require.Equal(t, 1, created, "exactly one concurrent send may create the message")
	require.Len(t, h.repo.messages, 1)
}

// --- 3. outsider access ----------------------------------------------------

func TestOutsiderCannotReadOrWriteAMatch(t *testing.T) {
	h := newHarness(t, nil)
	outsider := uuid.New()

	_, err := h.send(t, outsider, uuid.New(), "déjame entrar")
	require.ErrorIs(t, err, domainmessaging.ErrMatchNotAccessible)

	_, err = h.service.History(context.Background(), h.matchID, outsider, "", 0)
	require.ErrorIs(t, err, domainmessaging.ErrMatchNotAccessible)

	_, err = h.service.MarkRead(context.Background(), h.matchID, outsider)
	require.ErrorIs(t, err, domainmessaging.ErrMatchNotAccessible)
}

// --- 4. unmatch and block --------------------------------------------------

// The repository reports an unmatched or blocked conversation as inaccessible,
// so every operation must stop at that gate.
func TestUnmatchOrBlockStopsNewMessages(t *testing.T) {
	h := newHarness(t, nil)
	_, err := h.send(t, h.alice, uuid.New(), "antes")
	require.NoError(t, err)

	delete(h.repo.participants, h.matchID)

	_, err = h.send(t, h.alice, uuid.New(), "después")
	require.ErrorIs(t, err, domainmessaging.ErrMatchNotAccessible)

	_, err = h.service.History(context.Background(), h.matchID, h.alice, "", 0)
	require.ErrorIs(t, err, domainmessaging.ErrMatchNotAccessible)
}

// --- 5. stable cursor ------------------------------------------------------

func TestCursorPaginationHasNoGapsOrDuplicates(t *testing.T) {
	h := newHarness(t, nil)
	const total = 25
	for i := range total {
		_, err := h.send(t, h.alice, uuid.New(), "mensaje")
		require.NoError(t, err)
		_ = i
	}

	seen := map[uuid.UUID]bool{}
	cursor := ""
	pages := 0
	for {
		page, err := h.service.History(context.Background(), h.matchID, h.alice, cursor, 10)
		require.NoError(t, err)
		for _, message := range page.Messages {
			require.False(t, seen[message.ID], "cursor returned a duplicate message")
			seen[message.ID] = true
		}
		pages++
		require.Less(t, pages, 10, "pagination did not terminate")
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	require.Len(t, seen, total, "cursor skipped messages")
}

func TestHistoryRejectsATamperedCursor(t *testing.T) {
	h := newHarness(t, nil)
	_, err := h.service.History(context.Background(), h.matchID, h.alice, "not-a-cursor", 0)
	require.ErrorIs(t, err, domainmessaging.ErrInvalidCursor)
}

func TestHistoryRejectsAnOversizedPage(t *testing.T) {
	h := newHarness(t, nil)
	_, err := h.service.History(context.Background(), h.matchID, h.alice, "", 5000)
	require.ErrorIs(t, err, domainmessaging.ErrInvalidPageSize)
}

// --- 6/12. ticket lifecycle ------------------------------------------------

func TestTicketIsAcceptedOnceAndThenRejected(t *testing.T) {
	h := newHarness(t, nil)

	ticket, err := h.service.IssueTicket(context.Background(), h.alice)
	require.NoError(t, err)

	owner, err := h.service.ConsumeTicket(context.Background(), ticket.Value)
	require.NoError(t, err)
	require.Equal(t, h.alice, owner)

	_, err = h.service.ConsumeTicket(context.Background(), ticket.Value)
	require.ErrorIs(t, err, domainmessaging.ErrTicketInvalid, "a ticket must not be reusable")
}

func TestTamperedOrEmptyTicketIsRejected(t *testing.T) {
	h := newHarness(t, nil)
	ticket, err := h.service.IssueTicket(context.Background(), h.alice)
	require.NoError(t, err)

	_, err = h.service.ConsumeTicket(context.Background(), ticket.Value+"x")
	require.ErrorIs(t, err, domainmessaging.ErrTicketInvalid)

	_, err = h.service.ConsumeTicket(context.Background(), "   ")
	require.ErrorIs(t, err, domainmessaging.ErrTicketInvalid)
}

// TestRedisDownBlocksTicketIssueAndConsume is the fail-closed contract: an
// unreachable Redis must never be read as "allowed".
func TestRedisDownBlocksTicketIssueAndConsume(t *testing.T) {
	h := newHarness(t, nil)
	h.tickets.issueErr = errors.New("redis unavailable")

	_, err := h.service.IssueTicket(context.Background(), h.alice)
	require.ErrorIs(t, err, domainmessaging.ErrDependencyUnavailable)

	h.tickets.issueErr = nil
	h.tickets.failHard = errors.New("redis unavailable")
	_, err = h.service.ConsumeTicket(context.Background(), "whatever")
	require.Error(t, err)
	require.NotErrorIs(t, err, domainmessaging.ErrTicketInvalid,
		"a Redis outage must not be reported as a merely invalid ticket")
}

// --- 7. persist before publish ---------------------------------------------

// TestMessageIsDurableBeforeItIsPublished proves the ordering the plan
// demands: at the moment the event is published the row already exists.
func TestMessageIsDurableBeforeItIsPublished(t *testing.T) {
	h := newHarness(t, nil)
	storedAtPublishTime := -1
	h.publisher.onPublish = func() {
		h.repo.mu.Lock()
		storedAtPublishTime = len(h.repo.messages)
		h.repo.mu.Unlock()
	}

	_, err := h.send(t, h.alice, uuid.New(), "durable primero")
	require.NoError(t, err)
	require.Equal(t, 1, storedAtPublishTime, "the message must already be persisted when it is published")
}

func TestNothingIsPublishedWhenPersistenceFails(t *testing.T) {
	h := newHarness(t, nil)
	h.repo.sendErr = errors.New("database down")

	_, err := h.send(t, h.alice, uuid.New(), "no debe salir")
	require.Error(t, err)
	require.Equal(t, 0, h.publisher.count(), "a message that was never stored must never be broadcast")
}

// A fan-out failure does not fail the send: the message is durable and the
// client recovers live delivery over HTTP.
func TestPublishFailureStillReportsASuccessfulSend(t *testing.T) {
	h := newHarness(t, nil)
	h.publisher.err = errors.New("redis pubsub down")

	result, err := h.send(t, h.alice, uuid.New(), "persistido igualmente")
	require.NoError(t, err)
	require.True(t, result.Created)
	require.Len(t, h.repo.messages, 1)
}

// --- 10. limits ------------------------------------------------------------

func TestRateLimitedSenderIsRejectedWithRetryAfter(t *testing.T) {
	h := newHarness(t, fakeLimiter{allowed: false, retryAfter: 42 * time.Second})

	_, err := h.send(t, h.alice, uuid.New(), "demasiado rápido")
	var rateLimited *applicationmessaging.RateLimitedError
	require.ErrorAs(t, err, &rateLimited)
	require.Equal(t, 42*time.Second, rateLimited.RetryAfter)
	require.Empty(t, h.repo.messages)
}

func TestRateLimiterOutageFailsClosed(t *testing.T) {
	h := newHarness(t, fakeLimiter{err: errors.New("redis unavailable")})

	_, err := h.send(t, h.alice, uuid.New(), "sin límite comprobable")
	require.ErrorIs(t, err, domainmessaging.ErrDependencyUnavailable)
	require.Empty(t, h.repo.messages, "a send must not go through when the limit cannot be checked")
}

func TestOversizedAndEmptyContentAreRejected(t *testing.T) {
	h := newHarness(t, nil)

	oversized := make([]rune, domainmessaging.MaxContentLength+1)
	for i := range oversized {
		oversized[i] = 'a'
	}
	_, err := h.send(t, h.alice, uuid.New(), string(oversized))
	require.ErrorIs(t, err, domainmessaging.ErrContentTooLong)

	_, err = h.send(t, h.alice, uuid.New(), "   ")
	require.ErrorIs(t, err, domainmessaging.ErrEmptyContent)

	require.Empty(t, h.repo.messages)
}

func TestSendRequiresAClientNonce(t *testing.T) {
	h := newHarness(t, nil)
	_, err := h.send(t, h.alice, uuid.Nil, "sin nonce")
	require.ErrorIs(t, err, domainmessaging.ErrInvalidNonce)
}

// --- 11. read receipts -----------------------------------------------------

func TestMarkReadIsAuthorizedAndIdempotent(t *testing.T) {
	h := newHarness(t, nil)
	_, err := h.send(t, h.alice, uuid.New(), "para bob")
	require.NoError(t, err)

	updated, err := h.service.MarkRead(context.Background(), h.matchID, h.bob)
	require.NoError(t, err)
	require.Equal(t, 1, updated)

	updated, err = h.service.MarkRead(context.Background(), h.matchID, h.bob)
	require.NoError(t, err)
	require.Equal(t, 0, updated, "marking read again must change nothing")
}

func TestMarkReadNeverFlagsYourOwnMessages(t *testing.T) {
	h := newHarness(t, nil)
	_, err := h.send(t, h.alice, uuid.New(), "mío")
	require.NoError(t, err)

	updated, err := h.service.MarkRead(context.Background(), h.matchID, h.alice)
	require.NoError(t, err)
	require.Equal(t, 0, updated, "a sender cannot mark their own message as read")
}

// --- fan-out payload -------------------------------------------------------

// The broadcast must reach both participants and carry no credential.
func TestPublishedEventTargetsBothParticipants(t *testing.T) {
	h := newHarness(t, nil)
	_, err := h.send(t, h.alice, uuid.New(), "para los dos")
	require.NoError(t, err)

	require.Equal(t, 1, h.publisher.count())
	event := h.publisher.published[0]
	require.ElementsMatch(t, []uuid.UUID{h.alice, h.bob}, event.Recipients)
	require.Equal(t, h.matchID, event.MatchID)
	require.Equal(t, "para los dos", event.Message.Content)
}
