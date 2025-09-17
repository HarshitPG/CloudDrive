package notifications

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestHubBroadcast verifies that a published BroadcastMessage is delivered to registered clients.
func TestHubBroadcast(t *testing.T) {
	hub := NewHub()

	client := &Client{
		hub:  hub,
		send: make(chan BroadcastMessage, 1),
	}
	hub.Register(client)

	msg := BroadcastMessage{FileID: "4e85994a-e628-439d-9c4a-f01e4bba786c", DownloadCount: 5}
	hub.Publish(msg)

	select {
	case out := <-client.send:
		assert.Equal(t, msg.FileID, out.FileID)
		assert.Equal(t, msg.DownloadCount, out.DownloadCount)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for broadcast")
	}

	hub.Unregister(client)
	close(client.send)
	_ = hub.Shutdown(context.Background())
}
