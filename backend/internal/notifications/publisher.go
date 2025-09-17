package notifications

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
)

// PublishDownload publishes a download count update to Redis channel.
// Using Redis means any node can publish an event and all nodes will relay to connected WS clients.
func PublishDownload(ctx context.Context, rdb *redis.Client, fileID string, downloadCount int64) error {
	if rdb == nil {
		return nil
	}
	msg := BroadcastMessage{
		FileID:        fileID,
		DownloadCount: downloadCount,
		Timestamp:     time.Now().Unix(),
	}
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	// Use a short timeout so publishes don't hang.
	pctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	return rdb.Publish(pctx, downloadsChannel, b).Err()
}
