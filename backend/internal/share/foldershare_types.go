package share

import "time"

const (
	maxFoldersPerQuery = 500
	maxFilesPerQuery   = 1000
	maxDirectItems     = 100
	presignedURLTTL    = 15 * time.Minute
)

type CreateFolderShareReq struct {
	FolderID     string     `json:"folderId" binding:"required"`
	Title        string     `json:"title"`
	Description  string     `json:"description"`
	Recursive    bool       `json:"recursive"`
	SnapshotMode bool       `json:"snapshotMode"`
	ExpiresAt    *time.Time `json:"expiresAt"`
}

type FolderShareResponse struct {
	ID           string     `json:"id"`
	Token        string     `json:"token"`
	URL          string     `json:"url"`
	FolderID     string     `json:"folderId"`
	FolderName   string     `json:"folderName"`
	Title        string     `json:"title"`
	Description  string     `json:"description"`
	Recursive    bool       `json:"recursive"`
	SnapshotMode bool       `json:"snapshotMode"`
	ExpiresAt    *time.Time `json:"expiresAt"`
	CreatedAt    time.Time  `json:"createdAt"`
}

type ShareItem struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Type     string  `json:"type"`
	Size     *int64  `json:"size,omitempty"`
	MimeType string  `json:"mimeType,omitempty"`
	Path     string  `json:"path"`
	ParentID *string `json:"parentId,omitempty"`
}

type FolderShareListing struct {
	Share   FolderShareResponse `json:"share"`
	Items   []ShareItem         `json:"items"`
	Total   int                 `json:"total"`
	HasMore bool                `json:"hasMore"`
}

type folderShareRecord struct {
	ID           string
	Token        string
	CreatorID    string
	FolderID     string
	FolderName   string
	Title        string
	Description  string
	Recursive    bool
	SnapshotMode bool
	ExpiresAt    *time.Time
	CreatedAt    time.Time
}
