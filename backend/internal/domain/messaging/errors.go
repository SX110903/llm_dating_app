package messaging

import "errors"

var (
	ErrMessageNotFound = errors.New("message not found")
	// ErrMatchNotAccessible covers a match that does not exist, was unmatched,
	// involves a block, or simply does not belong to the caller. They are
	// deliberately indistinguishable so probing cannot reveal which one it is.
	ErrMatchNotAccessible = errors.New("match is not accessible")
	ErrInvalidMessageType = errors.New("invalid message type")
	ErrEmptyContent       = errors.New("text messages require content")
	ErrContentTooLong     = errors.New("message content exceeds the maximum length")
	ErrMediaKeyRequired   = errors.New("media messages require a storage key")
	ErrInvalidCursor      = errors.New("invalid message cursor")
	ErrInvalidPageSize    = errors.New("invalid page size")
	ErrInvalidNonce       = errors.New("invalid client nonce")
	// ErrDependencyUnavailable keeps the fail-closed doctrine of phase 1: when
	// Redis cannot be reached, tickets are neither issued nor consumed.
	ErrDependencyUnavailable = errors.New("messaging dependency unavailable")
	ErrTicketInvalid         = errors.New("websocket ticket is invalid or already used")
	ErrMessageTooFrequent    = errors.New("message rate limit exceeded")
)
