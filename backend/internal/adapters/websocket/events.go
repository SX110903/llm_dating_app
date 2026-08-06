package websocket

import (
	"time"

	"github.com/google/uuid"

	applicationmessaging "github.com/sx110903/llmatch-v2/backend/internal/application/messaging"
)

// Subprotocol is the value the browser must offer in Sec-WebSocket-Protocol
// alongside the ticket, so credentials never travel in the URL.
const Subprotocol = "llmatch.v1"

type ServerEventType string

const (
	EventMessage ServerEventType = "message"
	EventTyping  ServerEventType = "typing"
	EventClosed  ServerEventType = "conversation_closed"
	EventReady   ServerEventType = "ready"
)

// ServerEvent is what the backend sends down the socket.
type ServerEvent struct {
	Type    ServerEventType                      `json:"type"`
	Message *applicationmessaging.MessagePayload `json:"message,omitempty"`
	MatchID *uuid.UUID                           `json:"match_id,omitempty"`
	UserID  *uuid.UUID                           `json:"user_id,omitempty"`
	SentAt  time.Time                            `json:"sent_at"`
}

type ClientEventType string

const (
	ClientTyping ClientEventType = "typing"
	ClientPing   ClientEventType = "ping"
)

// ClientEvent is what the browser may send. Messages are deliberately not
// accepted here: sending goes through the authenticated HTTP endpoint, which
// keeps persistence and idempotency in one place.
type ClientEvent struct {
	Type    ClientEventType `json:"type"`
	MatchID uuid.UUID       `json:"match_id,omitempty"`
}
