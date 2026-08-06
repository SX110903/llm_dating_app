package websocket

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// newTestClient builds a client without a real connection: Enqueue and the
// queue policy are what these tests exercise, and neither touches the socket
// until the queue overflows.
func newTestClient(userID uuid.UUID, queueSize int) *Client {
	client, _ := newRecordingClient(userID, queueSize)
	return client
}

// newRecordingClient exposes whether the queue policy severed the connection,
// without putting any test state on the production type.
func newRecordingClient(userID uuid.UUID, queueSize int) (*Client, *atomic.Bool) {
	disconnected := &atomic.Bool{}
	client := &Client{
		userID:    userID,
		send:      make(chan ServerEvent, queueSize),
		closeOnce: make(chan struct{}),
	}
	client.disconnect = func(string) { disconnected.Store(true) }
	return client, disconnected
}

func TestHubDeliversOnlyToTheListedRecipients(t *testing.T) {
	hub := NewHub()
	alice, bob, carol := uuid.New(), uuid.New(), uuid.New()

	aliceClient := newTestClient(alice, 4)
	bobClient := newTestClient(bob, 4)
	carolClient := newTestClient(carol, 4)
	hub.Register(aliceClient)
	hub.Register(bobClient)
	hub.Register(carolClient)

	hub.Deliver([]uuid.UUID{alice, bob}, ServerEvent{Type: EventMessage, SentAt: time.Now()})

	require.Len(t, aliceClient.send, 1)
	require.Len(t, bobClient.send, 1)
	require.Empty(t, carolClient.send, "a user outside the conversation must receive nothing")
}

func TestHubStopsDeliveringAfterUnregister(t *testing.T) {
	hub := NewHub()
	alice := uuid.New()
	client := newTestClient(alice, 4)

	hub.Register(client)
	hub.Unregister(client)
	hub.Deliver([]uuid.UUID{alice}, ServerEvent{Type: EventMessage, SentAt: time.Now()})

	require.Empty(t, client.send)
}

func TestHubFansOutToEveryConnectionOfAUser(t *testing.T) {
	hub := NewHub()
	alice := uuid.New()
	phone := newTestClient(alice, 4)
	laptop := newTestClient(alice, 4)
	hub.Register(phone)
	hub.Register(laptop)

	hub.Deliver([]uuid.UUID{alice}, ServerEvent{Type: EventMessage, SentAt: time.Now()})

	require.Len(t, phone.send, 1)
	require.Len(t, laptop.send, 1)
}

// TestSlowClientIsDroppedInsteadOfBlockingTheHub is the backpressure contract:
// a consumer that cannot keep up is disconnected, and a full queue never
// stalls delivery to everybody else.
func TestSlowClientIsDroppedInsteadOfBlockingTheHub(t *testing.T) {
	hub := NewHub()
	slow, healthy := uuid.New(), uuid.New()
	slowClient, slowDisconnected := newRecordingClient(slow, 2)
	healthyClient := newTestClient(healthy, 16)
	hub.Register(slowClient)
	hub.Register(healthyClient)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 8 {
			hub.Deliver([]uuid.UUID{slow, healthy}, ServerEvent{Type: EventMessage, SentAt: time.Now()})
		}
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("a slow consumer blocked the hub")
	}

	require.Len(t, healthyClient.send, 8, "the healthy client must still receive everything")
	require.True(t, slowDisconnected.Load(), "the slow client should have been disconnected once its queue overflowed")
}

func TestEnqueueOnAClosedClientIsANoOp(t *testing.T) {
	client := newTestClient(uuid.New(), 2)
	close(client.closeOnce)

	require.NotPanics(t, func() {
		client.Enqueue(ServerEvent{Type: EventMessage, SentAt: time.Now()})
	})
	require.Empty(t, client.send)
}

func TestFrameLimiterAllowsUpToTheCapAndThenRejects(t *testing.T) {
	limiter := newFrameLimiter(3, time.Minute)
	now := time.Now()

	require.True(t, limiter.allow(now))
	require.True(t, limiter.allow(now))
	require.True(t, limiter.allow(now))
	require.False(t, limiter.allow(now), "the fourth frame in the window must be rejected")

	// A new window resets the budget.
	require.True(t, limiter.allow(now.Add(time.Minute+time.Second)))
}

func TestTicketIsReadFromTheSubprotocolHeaderOnly(t *testing.T) {
	request := newRequestWithProtocols("llmatch.v1, abc123")
	require.Equal(t, "abc123", ticketFromProtocols(request))

	require.Empty(t, ticketFromProtocols(newRequestWithProtocols("llmatch.v1")),
		"a handshake without a ticket must yield nothing")
}

func TestStripSchemeProducesHostPatterns(t *testing.T) {
	require.Equal(t, "app.example.com", stripScheme("https://app.example.com"))
	require.Equal(t, "localhost:5173", stripScheme("http://localhost:5173"))
	require.Equal(t, "localhost:5173", stripScheme("localhost:5173"))
}

func newRequestWithProtocols(value string) *http.Request {
	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/ws", nil)
	request.Header.Set("Sec-WebSocket-Protocol", value)
	return request
}
