package uploads

import (
	"context"
	"database/sql"

	"backend/internal/cache"
	"backend/internal/storage"
)

type Service interface {
	FolderInit(ctx context.Context, userID, parentID, rootName string, files []FolderInitFile) (FolderInitResponse, error)
	CreateSession(ctx context.Context, userID string, req CreateSessionRequest) (CreateSessionResponse, error)
	Complete(ctx context.Context, userID string, req CompleteRequest) (CompleteResponse, error)
	Abort(ctx context.Context, userID, sessionID string) error
}

type service struct {
	db      *sql.DB
	storage *storage.MinioStorage
	cache   cache.Cache
}

func New(db *sql.DB, st *storage.MinioStorage, c cache.Cache) Service {
	return &service{db: db, storage: st, cache: c}
}

type FolderInitFile struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	Mime   string `json:"mime"`
	SHA256 string `json:"sha256"`
}

type FolderInitResponse struct {
	UploadID     string                   `json:"uploadId"`
	RootFolderID string                   `json:"rootFolderId"`
	Folders      []map[string]string      `json:"folders"`
	Files        []FolderInitFileResponse `json:"files"`
}

type FolderInitFileResponse struct {
	Path           string `json:"path"`
	Deduped        bool   `json:"deduped"`
	UserFileID     string `json:"userFileId,omitempty"`
	SessionID      string `json:"sessionId,omitempty"`
	UploadUrl      string `json:"uploadUrl,omitempty"`
	TempBlobKey    string `json:"tempBlobKey,omitempty"`
	TargetFolderID string `json:"targetFolderId,omitempty"`
}

type CreateSessionRequest struct {
	Filename     string `json:"filename"`
	DeclaredMime string `json:"declaredMime"`
	OriginalSize int64  `json:"originalSize"`
	ClientSha256 string `json:"clientSha256"`
}

type CreateSessionResponse struct {
	SessionId      string `json:"sessionId,omitempty"`
	UploadUrl      string `json:"uploadUrl,omitempty"`
	TempBlobKey    string `json:"tempBlobKey,omitempty"`
	SkipUpload     bool   `json:"skipUpload"`
	ExistingFileId string `json:"existingFileId,omitempty"`
	UserFileId     string `json:"userFileId,omitempty"`
}

type CompleteRequest struct {
	SessionId    string `json:"sessionId"`
	ClientSha256 string `json:"clientSha256"`
	FolderID     string `json:"folderId"`
}

type CompleteResponse struct {
	UserFileID string `json:"userFileId"`
	ContentID  string `json:"contentId,omitempty"`
	Deduped    bool   `json:"deduped"`
}
