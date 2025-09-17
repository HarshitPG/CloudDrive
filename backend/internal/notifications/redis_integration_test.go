//go:build integration
// +build integration

package notifications

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// TestRedisPubSubIntegration validates that a message published to the Redis downloads channel
// is forwarded through the Hub to connected clients.
func TestRedisPubSubIntegration(t *testing.T) {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	defer rdb.Close()

	hub := NewHub()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	StartRedisSubscriber(ctx, rdb, hub)

	client := &Client{hub: hub, send: make(chan BroadcastMessage, 1)}
	hub.Register(client)

	event := BroadcastMessage{FileID: "f123", DownloadCount: 42}
	data, _ := json.Marshal(event)
	// publish raw JSON matching BroadcastMessage format to the channel the subscriber listens to
	require.NoError(t, rdb.Publish(ctx, downloadsChannel, data).Err())

	select {
	case out := <-client.send:
		require.Equal(t, event.FileID, out.FileID)
		require.Equal(t, event.DownloadCount, out.DownloadCount)
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for Redis forwarded event")
	}

	hub.Unregister(client)
	close(client.send)
	_ = hub.Shutdown(context.Background())
}
