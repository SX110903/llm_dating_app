package messaging

// SendMessageRequest deliberately has no storage key: object keys are always
// generated server-side, never accepted from a client. Only text is sendable
// in this phase, so "type" is constrained accordingly.
type SendMessageRequest struct {
	ClientNonce string `json:"client_nonce" validate:"required,uuid4"`
	Type        string `json:"type" validate:"required,oneof=text"`
	Content     string `json:"content" validate:"required,max=2000"`
}

type MessageResponse struct {
	ID       string `json:"id"`
	MatchID  string `json:"match_id"`
	SenderID string `json:"sender_id"`
	// Omitted in projections that do not carry it, such as the last message of
	// a conversation summary, so the contract never reports a fake nonce.
	ClientNonce string  `json:"client_nonce,omitempty"`
	Type        string  `json:"type"`
	Content     string  `json:"content,omitempty"`
	StorageKey  string  `json:"storage_key,omitempty"`
	ReadAt      *string `json:"read_at"`
	CreatedAt   string  `json:"created_at"`
}

type HistoryResponse struct {
	Messages   []MessageResponse `json:"messages"`
	NextCursor string            `json:"next_cursor,omitempty"`
}

type ConversationResponse struct {
	MatchID        string           `json:"match_id"`
	OtherUserID    string           `json:"other_user_id"`
	DisplayName    string           `json:"display_name"`
	PrimaryPhotoID string           `json:"primary_photo_id,omitempty"`
	LastMessage    *MessageResponse `json:"last_message,omitempty"`
	UnreadCount    int              `json:"unread_count"`
	MatchedAt      string           `json:"matched_at"`
}

type ConversationListResponse struct {
	Conversations []ConversationResponse `json:"conversations"`
}

type MarkReadResponse struct {
	Updated int `json:"updated"`
}

type TicketResponse struct {
	Ticket      string `json:"ticket"`
	ExpiresAt   string `json:"expires_at"`
	Subprotocol string `json:"subprotocol"`
}

type ErrorResponse struct {
	Code      string            `json:"code"`
	Message   string            `json:"message"`
	RequestID string            `json:"request_id"`
	Details   map[string]string `json:"details,omitempty"`
}
