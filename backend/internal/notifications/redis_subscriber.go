package notifications

import (
	"context"
	"encoding/json"
	"time"

	"backend/pkg/logger"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// RedisSubscriber subscribes to a Redis channel and pushes inbound messages to the Hub.

const downloadsChannel = "downloads:updates"

// StartRedisSubscriber starts a goroutine that subscribes to Redis and forwards messages to hub.
// It will automatically attempt to resubscribe on errors (basic retry/backoff).
func StartRedisSubscriber(ctx context.Context, rdb *redis.Client, hub *Hub) {
	go func() {
		backoff := time.Second
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			pubsub := rdb.Subscribe(ctx, downloadsChannel)
			// Wait for subscription confirmation
			_, err := pubsub.Receive(ctx)
			if err != nil {
				logger.L.Warn("redis subscribe failed", zap.Error(err))
				_ = pubsub.Close()
				time.Sleep(backoff)
				if backoff < 30*time.Second {
					backoff *= 2
				}
				continue
			}
			// Reset backoff on success
			backoff = time.Second

			ch := pubsub.Channel()

			// read loop
			for {
				select {
				case <-ctx.Done():
					_ = pubsub.Close()
					return
				case msg, ok := <-ch:
					if !ok {
						_ = pubsub.Close()
						break
					}
					var bm BroadcastMessage
					if err := json.Unmarshal([]byte(msg.Payload), &bm); err != nil {
						logger.L.Warn("invalid download message", zap.Error(err))
						continue
					}
					// Add timestamp
					if bm.Timestamp == 0 {
						bm.Timestamp = time.Now().Unix()
					}
					hub.Publish(bm)
				}
			}
			// small delay before trying to resub
			time.Sleep(time.Second)
		}
	}()
}
