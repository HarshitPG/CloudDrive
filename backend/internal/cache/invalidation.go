package cache

import (
	"backend/pkg/logger"
	"context"

	"go.uber.org/zap"
)

func InvalidateFile(ctx context.Context, c Cache, fileID string) {
	if c == nil {
		return
	}
	key := FileMetadataKey(fileID)
	if err := c.Delete(ctx, key); err != nil {
		logger.L.Warn("failed to invalidate file cache", zap.String("fileID", fileID), zap.Error(err))
	}
}

func InvalidateFolder(ctx context.Context, c Cache, folderID string) {
	if c == nil {
		return
	}
	key := FolderContentsKey(folderID)
	if err := c.Delete(ctx, key); err != nil {
		logger.L.Warn("failed to invalidate folder cache", zap.String("folderID", folderID), zap.Error(err))
	}
}

func InvalidateShare(ctx context.Context, c Cache, token string) {
	if c == nil {
		return
	}
	key := ShareResolveKey(token)
	if err := c.Delete(ctx, key); err != nil {
		logger.L.Warn("failed to invalidate share cache", zap.String("token", token), zap.Error(err))
	}
}

func InvalidateSearch(ctx context.Context, c Cache, userID string) {
	if c == nil {
		return
	}
	prefix := "cache:search:" + vSearch + ":" + userID + ":"
	// Note: requires Redis SCAN. For large deployments, use async worker.
	logger.L.Debug("invalidate search prefix", zap.String("prefix", prefix))
	// leave as TODO: depends on your Redis lib support for Scan+Del
}
