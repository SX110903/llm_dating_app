package websocket

import (
	"sync"

	"github.com/google/uuid"
)

// Hub tracks the connections this instance owns, indexed by user. Cross
// instance delivery is Redis Pub/Sub's job: the hub only fans an event out to
// the sockets attached here.
type Hub struct {
	mu      sync.RWMutex
	clients map[uuid.UUID]map[*Client]struct{}
}

func NewHub() *Hub {
	return &Hub{clients: make(map[uuid.UUID]map[*Client]struct{})}
}

func (h *Hub) Register(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[client.userID] == nil {
		h.clients[client.userID] = make(map[*Client]struct{})
	}
	h.clients[client.userID][client] = struct{}{}
}

func (h *Hub) Unregister(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	connections, ok := h.clients[client.userID]
	if !ok {
		return
	}
	delete(connections, client)
	if len(connections) == 0 {
		delete(h.clients, client.userID)
	}
}

// Deliver sends the event to every connection of the listed users that is
// still allowed to see the conversation.
func (h *Hub) Deliver(recipients []uuid.UUID, event ServerEvent) {
	h.mu.RLock()
	targets := make([]*Client, 0, len(recipients))
	for _, userID := range recipients {
		for client := range h.clients[userID] {
			targets = append(targets, client)
		}
	}
	h.mu.RUnlock()

	for _, client := range targets {
		client.Enqueue(event)
	}
}

func (h *Hub) ConnectionCount(userID uuid.UUID) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients[userID])
}
