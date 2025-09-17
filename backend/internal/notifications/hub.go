package notifications

import (
	"context"
	"sync"
)

// BroadcastMessage is the payload broadcast to connected WS clients.
type BroadcastMessage struct {
	FileID        string `json:"fileId"`
	DownloadCount int64  `json:"downloadCount"`
	Timestamp     int64  `json:"ts,omitempty"`
}

// Hub is a per-node in-memory hub for managing WebSocket clients and sending broadcasts.
// It receives messages (usually from Redis subscriber) and fans out to connected clients.
type Hub struct {
	mu        sync.RWMutex
	clients   map[*Client]struct{}
	broadcast chan BroadcastMessage
	quit      chan struct{}
}

// NewHub creates a Hub and starts its run loop.
func NewHub() *Hub {
	h := &Hub{
		clients:   make(map[*Client]struct{}),
		broadcast: make(chan BroadcastMessage, 128),
		quit:      make(chan struct{}),
	}
	go h.Run()
	return h
}

func (h *Hub) Run() {
	// fan-out loop
	for {
		select {
		case msg := <-h.broadcast:
			h.mu.RLock()
			for c := range h.clients {
				// do not block hub: try send with select to avoid slow client blocking
				select {
				case c.send <- msg:
				default:
					// client's send channel full -> drop client
					go c.close()
				}
			}
			h.mu.RUnlock()
		case <-h.quit:
			// close all clients gracefully
			h.mu.Lock()
			for c := range h.clients {
				c.close()
			}
			h.clients = map[*Client]struct{}{}
			h.mu.Unlock()
			return
		}
	}
}

// Register adds a client to the hub
func (h *Hub) Register(c *Client) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
}

// Unregister removes a client from the hub
func (h *Hub) Unregister(c *Client) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
}

// Publish enqueues a message to be broadcast to all clients
func (h *Hub) Publish(msg BroadcastMessage) {
	select {
	case h.broadcast <- msg:
	default:
	}
}

// Shutdown closes the hub and stops broadcasting
func (h *Hub) Shutdown(ctx context.Context) error {
	close(h.quit)
	return nil
}
