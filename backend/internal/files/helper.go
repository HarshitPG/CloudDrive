package files

import (
	"backend/internal/cache"
	"backend/internal/storage"
	"context"
	"time"
)

// cache keys passthrough helpers
func fileMetadataKey(fileID string) string { return cache.FileMetadataKey(fileID) }

// adapter for MinioStorage into our ObjectStorage interface
type minioAdapter struct{ s *storage.MinioStorage }

func (m minioAdapter) PresignedGetURL(ctx context.Context, objectKey string, expiryMinutes int64) (string, error) {
	return m.s.PresignedGetURL(ctx, objectKey, expiryMinutes)
}
func (m minioAdapter) RemoveObject(ctx context.Context, bucket, objectKey string, opts any) error {
	return m.s.Client.RemoveObject(ctx, bucket, objectKey, storage.MinioRemoveOpts())
}
func (m minioAdapter) BucketName() string  { return m.s.Bucket }
func (m minioAdapter) ClientPresent() bool { return m.s != nil && m.s.Client != nil }

// ttl helper for cache
const fiveMinutes = 5 * time.Minute
