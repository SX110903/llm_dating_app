package messaging

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"

	domainmessaging "github.com/sx110903/llmatch-v2/backend/internal/domain/messaging"
)

const (
	cursorVersion   = 1
	defaultPageSize = 30
	maxPageSize     = 100
	ticketBytes     = 32
)

// RateLimitedError reports how long the caller must wait before sending again.
type RateLimitedError struct {
	RetryAfter time.Duration
}

func (e *RateLimitedError) Error() string {
	return fmt.Sprintf("message rate limit exceeded, retry after %s", e.RetryAfter)
}

type Config struct {
	TicketTTL time.Duration
}

// SendTextInput carries a plain-text message. Media never travels this way:
// it has its own entry point so a storage key can never be client-supplied.
type SendTextInput struct {
	MatchID     uuid.UUID
	SenderID    uuid.UUID
	ClientNonce uuid.UUID
	Content     string
}

// Media messages are intentionally not sendable yet. The schema already
// models them, but a safe upload path needs a stored MIME type, size limits
// and moderation, none of which this phase defines. Accepting a
// client-supplied storage key instead would let a sender point a message at
// an object it does not own, so the API rejects those types outright.

type HistoryPage struct {
	Messages   []domainmessaging.Message
	NextCursor string
}

type Ticket struct {
	Value     string
	ExpiresAt time.Time
}

type Service struct {
	messages    domainmessaging.Repository
	tickets     TicketStore
	publisher   Publisher
	rateLimiter RateLimiter
	ticketTTL   time.Duration
	now         func() time.Time
}

func NewService(
	messages domainmessaging.Repository,
	tickets TicketStore,
	publisher Publisher,
	rateLimiter RateLimiter,
	cfg Config,
) *Service {
	return &Service{
		messages:    messages,
		tickets:     tickets,
		publisher:   publisher,
		rateLimiter: rateLimiter,
		ticketTTL:   cfg.TicketTTL,
		now:         func() time.Time { return time.Now().UTC() },
	}
}

// Authorize resolves the participants of an active match the caller belongs
// to. Every other operation goes through it first.
func (s *Service) Authorize(ctx context.Context, matchID, viewerID uuid.UUID) (*domainmessaging.Participants, error) {
	return s.messages.GetActiveParticipants(ctx, matchID, viewerID)
}

// SendText persists a text message and only then publishes it. If persistence
// fails nothing is broadcast, so a client can never see a message that does
// not exist. A replayed nonce returns the stored row without republishing.
func (s *Service) SendText(ctx context.Context, in SendTextInput) (*domainmessaging.SendResult, error) {
	if in.ClientNonce == uuid.Nil {
		return nil, domainmessaging.ErrInvalidNonce
	}
	content := strings.TrimSpace(in.Content)
	if content == "" {
		return nil, domainmessaging.ErrEmptyContent
	}
	if len([]rune(content)) > domainmessaging.MaxContentLength {
		return nil, domainmessaging.ErrContentTooLong
	}

	participants, err := s.admit(ctx, in.MatchID, in.SenderID)
	if err != nil {
		return nil, err
	}

	return s.persistAndPublish(ctx, participants, &domainmessaging.Message{
		ID:          uuid.New(),
		MatchID:     in.MatchID,
		SenderID:    in.SenderID,
		ClientNonce: in.ClientNonce,
		Type:        domainmessaging.MessageText,
		Content:     content,
	})
}

// admit runs the two gates every send shares: match membership and rate limit.
func (s *Service) admit(ctx context.Context, matchID, senderID uuid.UUID) (*domainmessaging.Participants, error) {
	participants, err := s.messages.GetActiveParticipants(ctx, matchID, senderID)
	if err != nil {
		return nil, err
	}
	allowed, retryAfter, err := s.rateLimiter.Allow(ctx, senderID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", domainmessaging.ErrDependencyUnavailable, err)
	}
	if !allowed {
		return nil, &RateLimitedError{RetryAfter: retryAfter}
	}
	return participants, nil
}

// persistAndPublish enforces the ordering the plan requires: durable first,
// broadcast second.
func (s *Service) persistAndPublish(
	ctx context.Context,
	participants *domainmessaging.Participants,
	message *domainmessaging.Message,
) (*domainmessaging.SendResult, error) {
	result, err := s.messages.Send(ctx, message)
	if err != nil {
		return nil, fmt.Errorf("persist message: %w", err)
	}

	// Republishing a replay would deliver the same message twice to peers that
	// already have it, so only a genuine insert is fanned out.
	if result.Created {
		event := MessageEvent{
			MatchID:    message.MatchID,
			Recipients: []uuid.UUID{participants.UserLowID, participants.UserHighID},
			Message:    NewMessagePayload(result.Message),
		}
		// The message is already durable, so the send succeeded. Losing the
		// fan-out only costs live delivery, which the client recovers over HTTP
		// from its last cursor.
		_ = s.publisher.Publish(ctx, event)
	}
	return result, nil
}

func (s *Service) History(ctx context.Context, matchID, viewerID uuid.UUID, cursor string, limit int) (*HistoryPage, error) {
	if _, err := s.messages.GetActiveParticipants(ctx, matchID, viewerID); err != nil {
		return nil, err
	}

	size, err := normalizePageSize(limit)
	if err != nil {
		return nil, err
	}

	params := domainmessaging.HistoryParams{MatchID: matchID, Limit: size + 1}
	if cursor != "" {
		decoded, decodeErr := decodeCursor(cursor)
		if decodeErr != nil {
			return nil, decodeErr
		}
		params.Before = decoded
	}

	messages, err := s.messages.ListHistory(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list history: %w", err)
	}

	page := &HistoryPage{Messages: messages}
	if len(messages) > size {
		page.Messages = messages[:size]
		last := page.Messages[len(page.Messages)-1]
		page.NextCursor, err = encodeCursor(cursorPayload{
			Version: cursorVersion, CreatedAt: last.CreatedAt, MessageID: last.ID,
		})
		if err != nil {
			return nil, err
		}
	}
	return page, nil
}

// MarkRead is idempotent: running it again simply updates no rows.
func (s *Service) MarkRead(ctx context.Context, matchID, viewerID uuid.UUID) (int, error) {
	if _, err := s.messages.GetActiveParticipants(ctx, matchID, viewerID); err != nil {
		return 0, err
	}
	return s.messages.MarkRead(ctx, matchID, viewerID, s.now())
}

func (s *Service) Conversations(ctx context.Context, viewerID uuid.UUID, limit int) ([]domainmessaging.ConversationSummary, error) {
	size, err := normalizePageSize(limit)
	if err != nil {
		return nil, err
	}
	return s.messages.ListConversations(ctx, viewerID, size)
}

// IssueTicket mints a single-use handshake ticket. If Redis cannot store it,
// the request fails instead of handing out a ticket nobody can validate.
func (s *Service) IssueTicket(ctx context.Context, userID uuid.UUID) (*Ticket, error) {
	raw := make([]byte, ticketBytes)
	if _, err := io.ReadFull(rand.Reader, raw); err != nil {
		return nil, fmt.Errorf("generate ticket: %w", err)
	}
	value := base64.RawURLEncoding.EncodeToString(raw)

	if err := s.tickets.Issue(ctx, value, userID, s.ticketTTL); err != nil {
		return nil, fmt.Errorf("%w: %w", domainmessaging.ErrDependencyUnavailable, err)
	}
	return &Ticket{Value: value, ExpiresAt: s.now().Add(s.ticketTTL)}, nil
}

// ConsumeTicket redeems a ticket exactly once. Any failure, including Redis
// being unreachable, rejects the handshake.
func (s *Service) ConsumeTicket(ctx context.Context, ticket string) (uuid.UUID, error) {
	if strings.TrimSpace(ticket) == "" {
		return uuid.Nil, domainmessaging.ErrTicketInvalid
	}
	userID, err := s.tickets.Consume(ctx, ticket)
	if err != nil {
		return uuid.Nil, err
	}
	return userID, nil
}

func normalizePageSize(limit int) (int, error) {
	if limit == 0 {
		return defaultPageSize, nil
	}
	if limit < 0 || limit > maxPageSize {
		return 0, domainmessaging.ErrInvalidPageSize
	}
	return limit, nil
}

type cursorPayload struct {
	Version   int       `json:"v"`
	CreatedAt time.Time `json:"c"`
	MessageID uuid.UUID `json:"i"`
}

func encodeCursor(value cursorPayload) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func decodeCursor(encoded string) (*domainmessaging.Cursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, domainmessaging.ErrInvalidCursor
	}
	var payload cursorPayload
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return nil, domainmessaging.ErrInvalidCursor
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, domainmessaging.ErrInvalidCursor
	}
	if payload.Version != cursorVersion || payload.CreatedAt.IsZero() || payload.MessageID == uuid.Nil {
		return nil, domainmessaging.ErrInvalidCursor
	}
	return &domainmessaging.Cursor{CreatedAt: payload.CreatedAt.UTC(), MessageID: payload.MessageID}, nil
}
