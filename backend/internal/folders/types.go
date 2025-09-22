package folders

import (
	"context"
	"database/sql"
	"io"

	"backend/internal/cache"
	"backend/internal/storage"
	"backend/internal/worker"
)

type Service interface {
	ListPrimary(ctx context.Context, userID, parentID string, deleted bool, limit, offset int) (PagedFolders, error)
	Create(ctx context.Context, userID, parentID, name string) (string, error)
	ListContents(ctx context.Context, userID, folderID string) (FolderContents, error)
	Rename(ctx context.Context, userID, folderID, name string) error
	WriteArchive(ctx context.Context, userID, folderID string, recursive bool, w io.Writer) error
	Move(ctx context.Context, userID, folderID, targetParentID string) error
	ListFilesInFolder(ctx context.Context, userID, folderID string, limit, offset int) (PagedFiles, error)
	GetTree(ctx context.Context, userID, rootID string) (FolderTree, error)
	GetAncestors(ctx context.Context, userID, folderID string) ([]Ancestor, error)
	Delete(ctx context.Context, userID, folderID string, permanent bool) (string, error)
}

type service struct {
	db       *sql.DB
	cache    cache.Cache
	producer *worker.Producer
	storage  *storage.MinioStorage
}

func New(db *sql.DB, c cache.Cache, st *storage.MinioStorage, producer *worker.Producer) Service {
	return &service{db: db, cache: c, storage: st, producer: producer}
}

type FolderItem struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	CreatedAt string  `json:"createdAt"`
	UpdatedAt string  `json:"updatedAt"`
	DeletedAt *string `json:"deletedAt,omitempty"`
	Size      int64   `json:"size"`
}

type PagedFolders struct {
	Folders []FolderItem `json:"folders"`
	Limit   int          `json:"limit"`
	Offset  int          `json:"offset"`
}

type FileListItem struct {
	ID            string `json:"id"`
	Filename      string `json:"filename"`
	MIME          string `json:"mime"`
	Size          int64  `json:"size"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
	DownloadCount int64  `json:"downloadCount"`
	ContentHash   string `json:"contentHash"`
	PhysicalSize  int64  `json:"physicalSize"`
	RefCount      int64  `json:"refCount"`
	DedupSavings  int64  `json:"dedupSavings"`
}

type PagedFiles struct {
	Files  []FileListItem `json:"files"`
	Limit  int            `json:"limit"`
	Offset int            `json:"offset"`
}

type FolderContents struct {
	Folders []FolderChild `json:"folders"`
	Files   []FileChild   `json:"files"`
}

type FolderChild struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"createdAt"`
}

type FileChild struct {
	ID        string `json:"id"`
	Filename  string `json:"filename"`
	MIME      string `json:"mime"`
	Size      int64  `json:"size"`
	CreatedAt string `json:"createdAt"`
}

type FolderTree struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	CreatedAt string       `json:"createdAt"`
	Children  []FolderTree `json:"children"`
}

type Ancestor struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
