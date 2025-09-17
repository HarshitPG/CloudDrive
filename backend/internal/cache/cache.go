package cache

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"backend/pkg/logger"
)

var (
	ErrCacheMiss = errors.New("cache miss")
	ErrCacheDown = errors.New("cache backend unavailable")
)

// Cache is the abstract interface for caching layer.
// This makes it easy to swap Redis with in-memory, Memcached, etc.
type Cache interface {
	Get(ctx context.Context, key string, dest interface{}) error
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	Delete(ctx context.Context, keys ...string) error
	Exists(ctx context.Context, key string) (bool, error)
}

type RedisCache struct {
	client *redis.Client
}

func NewRedisCache(client *redis.Client) *RedisCache {
	return &RedisCache{client: client}
}

func maybeCompress(b []byte) ([]byte, bool) {
	if len(b) < 8*1024 {
		return b, false
	}
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	_, _ = w.Write(b)
	_ = w.Close()
	return buf.Bytes(), true
}

func maybeDecompress(b []byte) []byte {
	buf := bytes.NewReader(b)
	r, err := zlib.NewReader(buf)
	if err != nil {
		return b
	}
	defer r.Close()
	out, _ := io.ReadAll(r)
	return out
}

func (r *RedisCache) Get(ctx context.Context, key string, dest interface{}) error {
	ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	val, err := r.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return ErrCacheMiss
	}
	if err != nil {
		return errors.Join(ErrCacheDown, err)
	}
	// val = maybeDecompress(val)
	if err := json.Unmarshal(val, dest); err != nil {
		return err
	}

	return nil
}

func (r *RedisCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond) // longer for writes
	defer cancel()

	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	// b, compressed := maybeCompress(b)
	// if compressed {
	// 	logger.L.Debug("cache payload compressed", zap.String("key", key), zap.Int("size", len(b)))
	// }
	if err := r.client.Set(ctx, key, b, ttl).Err(); err != nil {
		logger.L.Warn("cache set failed", zap.String("key", key), zap.Error(err))
		return errors.Join(ErrCacheDown, err)
	}
	return nil
}

func (r *RedisCache) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	if err := r.client.Del(ctx, keys...).Err(); err != nil {
		logger.L.Warn("cache delete failed", zap.Strings("keys", keys), zap.Error(err))
		return errors.Join(ErrCacheDown, err)
	}
	return nil
}

func (r *RedisCache) Exists(ctx context.Context, key string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	val, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return false, errors.Join(ErrCacheDown, err)
	}
	return val > 0, nil
}
