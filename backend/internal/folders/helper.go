package folders

import (
	"backend/internal/cache"
	"time"
)

const (
	folderListTTL = cache.TTLFolderList
)

func folderContentsKey(folderID string) string { return cache.FolderContentsKey(folderID) }

func normalizeLimitOffset(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	if offset > 100000 {
		offset = 100000
	}
	return limit, offset
}

var fiveMinutes = 5 * time.Minute
