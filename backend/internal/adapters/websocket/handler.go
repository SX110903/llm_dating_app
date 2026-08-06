package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/rs/zerolog"

	applicationmessaging "github.com/sx110903/llmatch-v2/backend/internal/application/messaging"
	domainmessaging "github.com/sx110903/llmatch-v2/backend/internal/domain/messaging"
)

type Config struct {
	ReadLimitBytes        int64
	QueueSize             int
	WriteTimeout          time.Duration
	PingInterval          time.Duration
	ReadTimeout           time.Duration
	ClientEventsPerMinute int
}

type Handler struct {
	service        *applicationmessaging.Service
	hub            *Hub
	logger         zerolog.Logger
	allowedOrigins map[string]struct{}
	cfg            Config
}

func NewHandler(service *applicationmessaging.Service, hub *Hub, logger zerolog.Logger, allowedOrigins []string, cfg Config) *Handler {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		allowed[origin] = struct{}{}
	}
	return &Handler{service: service, hub: hub, logger: logger, allowedOrigins: allowed, cfg: cfg}
}

// ServeHTTP performs the handshake. The ticket arrives as the second
// Sec-WebSocket-Protocol entry, so no credential ever appears in the URL,
// where it would end up in logs and referrers.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ticket := ticketFromProtocols(r)
	if ticket == "" {
		http.Error(w, "missing websocket ticket", http.StatusUnauthorized)
		return
	}

	userID, err := h.service.ConsumeTicket(r.Context(), ticket)
	if err != nil {
		// A Redis outage must not be mistaken for a valid handshake.
		if errors.Is(err, domainmessaging.ErrDependencyUnavailable) {
			http.Error(w, "messaging dependency unavailable", http.StatusServiceUnavailable)
			return
		}
		http.Error(w, "invalid websocket ticket", http.StatusUnauthorized)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		Subprotocols:   []string{Subprotocol},
		OriginPatterns: h.originPatterns(),
	})
	if err != nil {
		return
	}
	conn.SetReadLimit(h.cfg.ReadLimitBytes)

	client := newClient(userID, conn, h.cfg.QueueSize, h.cfg.WriteTimeout, h.logger)
	h.hub.Register(client)
	defer func() {
		h.hub.Unregister(client)
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	go client.writePump(ctx, h.cfg.PingInterval)
	client.Enqueue(ServerEvent{Type: EventReady, SentAt: time.Now().UTC()})

	h.readPump(ctx, client)
}

// readPump handles inbound frames. Only ephemeral signals are accepted; the
// typing indicator is relayed and never stored.
func (h *Handler) readPump(ctx context.Context, client *Client) {
	limiter := newFrameLimiter(h.cfg.ClientEventsPerMinute, time.Minute)

	for {
		readCtx, cancel := context.WithTimeout(ctx, h.cfg.ReadTimeout)
		messageType, payload, err := client.conn.Read(readCtx)
		cancel()
		if err != nil {
			return
		}
		if messageType != websocket.MessageText {
			_ = client.conn.Close(websocket.StatusUnsupportedData, "text frames only")
			return
		}
		if !limiter.allow(time.Now()) {
			_ = client.conn.Close(websocket.StatusPolicyViolation, "too many client events")
			return
		}

		var event ClientEvent
		if err := json.Unmarshal(payload, &event); err != nil {
			continue
		}
		h.handleClientEvent(ctx, client, event)
	}
}

func (h *Handler) handleClientEvent(ctx context.Context, client *Client, event ClientEvent) {
	switch event.Type {
	case ClientPing:
		client.Enqueue(ServerEvent{Type: EventReady, SentAt: time.Now().UTC()})
	case ClientTyping:
		if event.MatchID == uuid.Nil {
			return
		}
		// Authorize every relay: an unmatch or block between frames must stop
		// the indicator immediately.
		participants, err := h.service.Authorize(ctx, event.MatchID, client.userID)
		if err != nil {
			client.Enqueue(ServerEvent{Type: EventClosed, MatchID: &event.MatchID, SentAt: time.Now().UTC()})
			return
		}
		other := participants.Other(client.userID)
		matchID := event.MatchID
		userID := client.userID
		h.hub.Deliver([]uuid.UUID{other}, ServerEvent{
			Type: EventTyping, MatchID: &matchID, UserID: &userID, SentAt: time.Now().UTC(),
		})
	}
}

// Broadcast is the entry point used by the Redis subscriber to fan an event
// out to the sockets this instance owns.
func (h *Handler) Broadcast(event applicationmessaging.MessageEvent) {
	payload := event.MessageJSON
	matchID := event.MatchID
	h.hub.Deliver(event.Recipients, ServerEvent{
		Type: EventMessage, Message: &payload, MatchID: &matchID, SentAt: time.Now().UTC(),
	})
}

func (h *Handler) originPatterns() []string {
	patterns := make([]string, 0, len(h.allowedOrigins))
	for origin := range h.allowedOrigins {
		patterns = append(patterns, stripScheme(origin))
	}
	return patterns
}

// stripScheme turns an allowlisted origin into the host pattern the websocket
// library expects for its own Origin check.
func stripScheme(origin string) string {
	for _, prefix := range []string{"https://", "http://"} {
		if after, found := strings.CutPrefix(origin, prefix); found {
			return after
		}
	}
	return origin
}

// ticketFromProtocols expects "llmatch.v1, <ticket>"; the browser API only
// lets a client pass extra data through this header.
func ticketFromProtocols(r *http.Request) string {
	for _, header := range r.Header.Values("Sec-WebSocket-Protocol") {
		for _, part := range strings.Split(header, ",") {
			if trimmed := strings.TrimSpace(part); trimmed != "" && trimmed != Subprotocol {
				return trimmed
			}
		}
	}
	return ""
}

// frameLimiter is a per-connection fixed window over inbound frames.
type frameLimiter struct {
	limit       int
	window      time.Duration
	windowStart time.Time
	count       int
}

func newFrameLimiter(limit int, window time.Duration) *frameLimiter {
	return &frameLimiter{limit: limit, window: window}
}

func (l *frameLimiter) allow(now time.Time) bool {
	if now.Sub(l.windowStart) >= l.window {
		l.windowStart = now
		l.count = 0
	}
	l.count++
	return l.count <= l.limit
}
