package files

import (
	"context"
	"database/sql"

	"backend/internal/cache"
	"backend/internal/storage"
	"backend/internal/worker"
)

type Service interface {
	GetDownloadURL(ctx context.Context, userID, fileID string) (string, error)
	Delete(ctx context.Context, userID, fileID string, permanent bool) (string, error)
	Restore(ctx context.Context, userID, fileID string) (string, error)
	Patch(ctx context.Context, userID, fileID string, req PatchRequest) error
	Move(ctx context.Context, userID, fileID string, targetFolderID string) error
	CreateVersion(ctx context.Context, userID, fileID string, payload CreateVersionRequest) error
	ListVersions(ctx context.Context, fileID string) ([]FileVersion, error)
	GetMetadata(ctx context.Context, userID, fileID string) (FileMetadata, error)
	ListFiles(ctx context.Context, userID, folderID string, deleted bool) ([]FileListItem, error)
	ListFilesPrimary(ctx context.Context, userID, folderID string, deleted bool) ([]FileListItem, error)
}

type service struct {
	db       *sql.DB
	storage  *storage.MinioStorage
	cache    cache.Cache
	publish  func(ctx context.Context, fileID string, downloadCount int64) error
	producer *worker.Producer
}

func New(db *sql.DB, st *storage.MinioStorage, c cache.Cache, publish func(ctx context.Context, fileID string, downloadCount int64) error, producer *worker.Producer) Service {
	return &service{db: db, storage: st, cache: c, publish: publish, producer: producer}
}

type PatchRequest struct {
	Filename string   `json:"filename"`
	Tags     []string `json:"tags"`
}

type CreateVersionRequest struct {
	ContentID string `json:"content_id"`
	Filename  string `json:"filename"`
}

type FileVersion struct {
	ID        string `json:"id"`
	ContentID string `json:"content_id"`
	Filename  string `json:"filename"`
	CreatedAt string `json:"created_at"`
}

type FileMetadata struct {
	Filename      string  `json:"filename"`
	MIME          string  `json:"mime"`
	Size          int64   `json:"size"`
	CreatedAt     string  `json:"createdAt"`
	UpdatedAt     string  `json:"updatedAt"`
	DownloadCount int64   `json:"downloadCount"`
	ContentHash   string  `json:"contentHash"`
	PhysicalSize  int64   `json:"physicalSize"`
	RefCount      int64   `json:"refCount"`
	FolderID      *string `json:"folderId"`
	DedupSavings  int64   `json:"dedupSavings"`
}

type FileListItem struct {
	ID            string  `json:"id"`
	Filename      string  `json:"filename"`
	MIME          string  `json:"mime"`
	Size          int64   `json:"size"`
	CreatedAt     string  `json:"createdAt"`
	UpdatedAt     string  `json:"updatedAt"`
	DownloadCount int64   `json:"downloadCount"`
	ContentHash   string  `json:"contentHash"`
	PhysicalSize  int64   `json:"physicalSize"`
	RefCount      int64   `json:"refCount"`
	DedupSavings  int64   `json:"dedupSavings"`
	DeletedAt     *string `json:"deletedAt,omitempty"`
}
