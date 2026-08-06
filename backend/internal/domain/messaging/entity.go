package messaging

import (
	"time"

	"github.com/google/uuid"
)

// MaxContentLength mirrors the varchar(2000) column: the database is the last
// line of defence, not the only one.
const MaxContentLength = 2000

type MessageType string

const (
	MessageText  MessageType = "text"
	MessageImage MessageType = "image"
	MessageGIF   MessageType = "gif"
)

func (t MessageType) IsValid() bool {
	return t == MessageText || t == MessageImage || t == MessageGIF
}

func (t MessageType) IsMedia() bool {
	return t == MessageImage || t == MessageGIF
}

// Message is one entry of a conversation. The recipient is never stored: it is
// derived from the match participants.
type Message struct {
	ID          uuid.UUID
	MatchID     uuid.UUID
	SenderID    uuid.UUID
	ClientNonce uuid.UUID
	Type        MessageType
	Content     string
	StorageKey  string
	ReadAt      *time.Time
	CreatedAt   time.Time
	DeletedAt   *time.Time
}

func (m Message) IsDeleted() bool { return m.DeletedAt != nil }
func (m Message) IsRead() bool    { return m.ReadAt != nil }

// Cursor is a keyset position over (created_at, id). Pagination never uses
// OFFSET, so inserts during paging cannot produce gaps or duplicates.
type Cursor struct {
	CreatedAt time.Time
	MessageID uuid.UUID
}

type HistoryParams struct {
	MatchID  uuid.UUID
	ViewerID uuid.UUID
	Limit    int
	// Before pages backwards through history, oldest-ward from the cursor.
	Before *Cursor
}

// SendResult reports whether the write created a row or matched an existing
// nonce, so the caller can tell a genuine send from an idempotent replay.
type SendResult struct {
	Message Message
	Created bool
}

// Participants are the two members of an active match, used to authorize and
// to fan out a delivered message.
type Participants struct {
	MatchID    uuid.UUID
	UserLowID  uuid.UUID
	UserHighID uuid.UUID
	MatchedAt  time.Time
}

func (p Participants) Includes(userID uuid.UUID) bool {
	return p.UserLowID == userID || p.UserHighID == userID
}

func (p Participants) Other(userID uuid.UUID) uuid.UUID {
	if p.UserLowID == userID {
		return p.UserHighID
	}
	return p.UserLowID
}

// ConversationSummary is the chat-list projection of an active match.
type ConversationSummary struct {
	MatchID        uuid.UUID
	OtherUserID    uuid.UUID
	DisplayName    string
	PrimaryPhotoID uuid.UUID
	LastMessage    *Message
	UnreadCount    int
	MatchedAt      time.Time
}
