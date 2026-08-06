package websocket

import (
	"context"
	"encoding/json"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// Client owns one socket. Outbound events go through a bounded queue: if a
// consumer is too slow to drain it, the connection is closed instead of
// letting the backlog grow without limit.
type Client struct {
	userID uuid.UUID
	conn   *websocket.Conn
	send   chan ServerEvent
	logger zerolog.Logger

	writeTimeout time.Duration
	closeOnce    chan struct{}
	// disconnect severs the socket when the queue policy decides the consumer
	// cannot keep up. Injected so that policy is exercisable on its own.
	disconnect func(reason string)
}

func newClient(userID uuid.UUID, conn *websocket.Conn, queueSize int, writeTimeout time.Duration, logger zerolog.Logger) *Client {
	return &Client{
		userID:       userID,
		conn:         conn,
		send:         make(chan ServerEvent, queueSize),
		logger:       logger,
		writeTimeout: writeTimeout,
		closeOnce:    make(chan struct{}),
		disconnect: func(reason string) {
			_ = conn.Close(websocket.StatusPolicyViolation, reason)
		},
	}
}

// Enqueue never blocks. A full queue means the client cannot keep up, so it is
// dropped: back-pressure is applied by disconnecting, not by stalling the hub.
func (c *Client) Enqueue(event ServerEvent) {
	select {
	case <-c.closeOnce:
		return
	case c.send <- event:
	default:
		c.logger.Warn().Str("user_id", c.userID.String()).Msg("websocket send queue full, closing connection")
		c.closeSlow()
	}
}

func (c *Client) closeSlow() {
	select {
	case <-c.closeOnce:
	default:
		close(c.closeOnce)
		c.disconnect("client too slow")
	}
}

// writePump drains the queue and keeps the connection alive with pings.
func (c *Client) writePump(ctx context.Context, pingInterval time.Duration) {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.closeOnce:
			return
		case event := <-c.send:
			payload, err := json.Marshal(event)
			if err != nil {
				continue
			}
			writeCtx, cancel := context.WithTimeout(ctx, c.writeTimeout)
			err = c.conn.Write(writeCtx, websocket.MessageText, payload)
			cancel()
			if err != nil {
				return
			}
		case <-ticker.C:
			pingCtx, cancel := context.WithTimeout(ctx, c.writeTimeout)
			err := c.conn.Ping(pingCtx)
			cancel()
			if err != nil {
				return
			}
		}
	}
}
