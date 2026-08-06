package messaging

import (
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/microcosm-cc/bluemonday"

	websocketadapter "github.com/sx110903/llmatch-v2/backend/internal/adapters/websocket"
	applicationmessaging "github.com/sx110903/llmatch-v2/backend/internal/application/messaging"
	domainmessaging "github.com/sx110903/llmatch-v2/backend/internal/domain/messaging"
	platformmiddleware "github.com/sx110903/llmatch-v2/backend/internal/platform/middleware"
)

const maxJSONBodyBytes = 64 * 1024

type Handler struct {
	service   *applicationmessaging.Service
	validate  *validator.Validate
	sanitizer *bluemonday.Policy
}

func NewHandler(service *applicationmessaging.Service) *Handler {
	return &Handler{
		service:   service,
		validate:  validator.New(validator.WithRequiredStructEnabled()),
		sanitizer: bluemonday.StrictPolicy(),
	}
}

func (h *Handler) ListConversations(w http.ResponseWriter, r *http.Request) {
	identity, ok := requireIdentity(w, r)
	if !ok {
		return
	}
	limit, err := parseLimit(r.URL.Query().Get("limit"))
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_PAGE_SIZE", "invalid limit")
		return
	}

	conversations, err := h.service.Conversations(r.Context(), identity.UserID, limit)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	response := ConversationListResponse{Conversations: make([]ConversationResponse, 0, len(conversations))}
	for _, conversation := range conversations {
		response.Conversations = append(response.Conversations, conversationResponse(conversation))
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) History(w http.ResponseWriter, r *http.Request) {
	identity, ok := requireIdentity(w, r)
	if !ok {
		return
	}
	matchID, ok := parseMatchID(w, r)
	if !ok {
		return
	}
	limit, err := parseLimit(r.URL.Query().Get("limit"))
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_PAGE_SIZE", "invalid limit")
		return
	}

	page, err := h.service.History(r.Context(), matchID, identity.UserID, r.URL.Query().Get("cursor"), limit)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	response := HistoryResponse{Messages: make([]MessageResponse, 0, len(page.Messages)), NextCursor: page.NextCursor}
	for _, message := range page.Messages {
		response.Messages = append(response.Messages, messageResponse(message))
	}
	writeJSON(w, http.StatusOK, response)
}

// Send is idempotent through client_nonce: a retry returns the stored message
// with 200 instead of creating a second one.
func (h *Handler) Send(w http.ResponseWriter, r *http.Request) {
	identity, ok := requireIdentity(w, r)
	if !ok {
		return
	}
	matchID, ok := parseMatchID(w, r)
	if !ok {
		return
	}

	var req SendMessageRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_BODY", "request body could not be parsed")
		return
	}
	if err := h.validate.Struct(req); err != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "request failed validation")
		return
	}
	nonce, err := uuid.Parse(req.ClientNonce)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid client nonce")
		return
	}

	// The sender always comes from the authenticated identity, never the body.
	result, err := h.service.SendText(r.Context(), applicationmessaging.SendTextInput{
		MatchID:     matchID,
		SenderID:    identity.UserID,
		ClientNonce: nonce,
		Content:     h.sanitize(req.Content),
	})
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	status := http.StatusOK
	if result.Created {
		status = http.StatusCreated
	}
	writeJSON(w, status, messageResponse(result.Message))
}

func (h *Handler) MarkRead(w http.ResponseWriter, r *http.Request) {
	identity, ok := requireIdentity(w, r)
	if !ok {
		return
	}
	matchID, ok := parseMatchID(w, r)
	if !ok {
		return
	}

	updated, err := h.service.MarkRead(r.Context(), matchID, identity.UserID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, MarkReadResponse{Updated: updated})
}

// IssueTicket hands out the single-use handshake ticket. It is authenticated
// like any other route, so the ticket inherits the caller's identity.
func (h *Handler) IssueTicket(w http.ResponseWriter, r *http.Request) {
	identity, ok := requireIdentity(w, r)
	if !ok {
		return
	}
	ticket, err := h.service.IssueTicket(r.Context(), identity.UserID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, TicketResponse{
		Ticket:      ticket.Value,
		ExpiresAt:   ticket.ExpiresAt.Format(time.RFC3339),
		Subprotocol: websocketadapter.Subprotocol,
	})
}

func requireIdentity(w http.ResponseWriter, r *http.Request) (platformmiddleware.Identity, bool) {
	identity, ok := platformmiddleware.IdentityFromContext(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "missing identity")
		return platformmiddleware.Identity{}, false
	}
	return identity, true
}

func parseMatchID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	matchID, err := uuid.Parse(chi.URLParam(r, "matchID"))
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid match id")
		return uuid.Nil, false
	}
	return matchID, true
}

func parseLimit(raw string) (int, error) {
	if raw == "" {
		return 0, nil
	}
	return strconv.Atoi(raw)
}

func (h *Handler) sanitize(value string) string {
	return strings.TrimSpace(h.sanitizer.Sanitize(value))
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dest any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
	defer func() { _ = r.Body.Close() }()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dest); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain one JSON object")
	}
	return nil
}

func writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	var rateLimited *applicationmessaging.RateLimitedError
	switch {
	case errors.As(err, &rateLimited):
		w.Header().Set("Retry-After", strconv.Itoa(max(1, int(math.Ceil(rateLimited.RetryAfter.Seconds())))))
		writeError(w, r, http.StatusTooManyRequests, "RATE_LIMITED", "too many messages, slow down")
	case errors.Is(err, domainmessaging.ErrDependencyUnavailable):
		writeError(w, r, http.StatusServiceUnavailable, "MESSAGING_DEPENDENCY_UNAVAILABLE", "messaging dependency unavailable")
	case errors.Is(err, domainmessaging.ErrMatchNotAccessible):
		// Unmatched, blocked, foreign or non-existent are all 404 so probing
		// cannot distinguish them.
		writeError(w, r, http.StatusNotFound, "MATCH_NOT_FOUND", "conversation not found")
	case errors.Is(err, domainmessaging.ErrInvalidCursor):
		writeError(w, r, http.StatusBadRequest, "INVALID_CURSOR", "invalid cursor")
	case errors.Is(err, domainmessaging.ErrInvalidPageSize):
		writeError(w, r, http.StatusBadRequest, "INVALID_PAGE_SIZE", "invalid limit")
	case errors.Is(err, domainmessaging.ErrContentTooLong):
		writeError(w, r, http.StatusUnprocessableEntity, "CONTENT_TOO_LONG", "message is too long")
	case errors.Is(err, domainmessaging.ErrEmptyContent):
		writeError(w, r, http.StatusUnprocessableEntity, "EMPTY_CONTENT", "text messages require content")
	case errors.Is(err, domainmessaging.ErrInvalidMessageType), errors.Is(err, domainmessaging.ErrInvalidNonce):
		writeError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "message payload is not coherent")
	default:
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "unexpected messaging error")
	}
}

func messageResponse(message domainmessaging.Message) MessageResponse {
	response := MessageResponse{
		ID:         message.ID.String(),
		MatchID:    message.MatchID.String(),
		SenderID:   message.SenderID.String(),
		Type:       string(message.Type),
		Content:    message.Content,
		StorageKey: message.StorageKey,
		CreatedAt:  message.CreatedAt.Format(time.RFC3339),
	}
	if message.ClientNonce != uuid.Nil {
		response.ClientNonce = message.ClientNonce.String()
	}
	if message.ReadAt != nil {
		readAt := message.ReadAt.Format(time.RFC3339)
		response.ReadAt = &readAt
	}
	return response
}

func conversationResponse(conversation domainmessaging.ConversationSummary) ConversationResponse {
	response := ConversationResponse{
		MatchID:     conversation.MatchID.String(),
		OtherUserID: conversation.OtherUserID.String(),
		DisplayName: conversation.DisplayName,
		UnreadCount: conversation.UnreadCount,
		MatchedAt:   conversation.MatchedAt.Format(time.RFC3339),
	}
	if conversation.PrimaryPhotoID != uuid.Nil {
		response.PrimaryPhotoID = conversation.PrimaryPhotoID.String()
	}
	if conversation.LastMessage != nil {
		last := messageResponse(*conversation.LastMessage)
		response.LastMessage = &last
	}
	return response
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponse{
		Code:      code,
		Message:   message,
		RequestID: platformmiddleware.RequestIDFromContext(r.Context()),
	})
}
