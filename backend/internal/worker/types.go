package worker

type GCJob struct {
	ContentID string `json:"contentId"`
	BlobKey   string `json:"blobKey"`
}

// FolderShareJob represents a folder share snapshot generation job
type FolderShareJob struct {
	ShareID      string `json:"shareId"`
	FolderID     string `json:"folderId"`
	Token        string `json:"token"`
	Recursive    bool   `json:"recursive"`
	SnapshotMode bool   `json:"snapshotMode"`
	CreatedAt    string `json:"createdAt"`
}

// FolderShareEvent represents events related to folder sharing
type FolderShareEvent struct {
	Type      string `json:"type"` // "created", "accessed", "revoked"
	ShareID   string `json:"shareId"`
	FolderID  string `json:"folderId"`
	Token     string `json:"token"`
	UserID    string `json:"userId,omitempty"`    // For created/revoked events
	IPAddress string `json:"ipAddress,omitempty"` // For access events
	Timestamp string `json:"timestamp"`
}
