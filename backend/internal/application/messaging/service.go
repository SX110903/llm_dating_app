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

type SendInput struct {
	MatchID     uuid.UUID
	SenderID    uuid.UUID
	ClientNonce uuid.UUID
	Type        domainmessaging.MessageType
	Content     string
	StorageKey  string
}

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

// Send persists the message and only then publishes it. If persistence fails
// nothing is broadcast, so a client can never see a message that does not
// exist. A replayed nonce returns the stored row without republishing.
func (s *Service) Send(ctx context.Context, in SendInput) (*domainmessaging.SendResult, error) {
	if err := validateSend(in); err != nil {
		return nil, err
	}

	participants, err := s.messages.GetActiveParticipants(ctx, in.MatchID, in.SenderID)
	if err != nil {
		return nil, err
	}

	allowed, retryAfter, err := s.rateLimiter.Allow(ctx, in.SenderID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", domainmessaging.ErrDependencyUnavailable, err)
	}
	if !allowed {
		return nil, &RateLimitedError{RetryAfter: retryAfter}
	}

	message := &domainmessaging.Message{
		ID:          uuid.New(),
		MatchID:     in.MatchID,
		SenderID:    in.SenderID,
		ClientNonce: in.ClientNonce,
		Type:        in.Type,
		Content:     in.Content,
		StorageKey:  in.StorageKey,
	}
	result, err := s.messages.Send(ctx, message)
	if err != nil {
		return nil, fmt.Errorf("persist message: %w", err)
	}

	// Republishing a replay would deliver the same message twice to peers that
	// already have it, so only a genuine insert is fanned out.
	if result.Created {
		event := MessageEvent{
			MatchID:     in.MatchID,
			Recipients:  []uuid.UUID{participants.UserLowID, participants.UserHighID},
			Message:     result.Message,
			MessageJSON: NewMessagePayload(result.Message),
		}
		if err := s.publisher.Publish(ctx, event); err != nil {
			// The message is already durable; a fan-out failure degrades to
			// "not delivered live", which the client recovers over HTTP.
			return result, nil
		}
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

	params := domainmessaging.HistoryParams{MatchID: matchID, ViewerID: viewerID, Limit: size + 1}
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

func validateSend(in SendInput) error {
	if !in.Type.IsValid() {
		return domainmessaging.ErrInvalidMessageType
	}
	if in.ClientNonce == uuid.Nil {
		return domainmessaging.ErrInvalidNonce
	}
	if in.Type == domainmessaging.MessageText {
		if strings.TrimSpace(in.Content) == "" {
			return domainmessaging.ErrEmptyContent
		}
		if len([]rune(in.Content)) > domainmessaging.MaxContentLength {
			return domainmessaging.ErrContentTooLong
		}
		if in.StorageKey != "" {
			return domainmessaging.ErrInvalidMessageType
		}
		return nil
	}
	if in.StorageKey == "" {
		return domainmessaging.ErrMediaKeyRequired
	}
	if in.Content != "" {
		return domainmessaging.ErrInvalidMessageType
	}
	return nil
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
