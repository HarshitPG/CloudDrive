package llmclient

import (
	"backend/internal/storage"
	"backend/pkg/logger"
	"context"
	"errors"
	"sync"
	"time"

	"go.uber.org/zap"
)

type Service interface {
	// Summary operations
	TriggerSummary(ctx context.Context, fileID, objectPath, filename string)
	GetSummary(ctx context.Context, fileID string) (*SummaryResult, error)

	// Chat operations
	ProcessForChat(ctx context.Context, fileID, objectPath, filename string) error
	GetChatStatus(ctx context.Context, fileID string) (*ChatProcessingResult, error)
	Chat(ctx context.Context, fileID, question string) (*ChatResponse, error)
	GetChatHistory(ctx context.Context, fileID string) (*ChatHistory, error)
	ClearChatHistory(ctx context.Context, fileID string) error

	// Cleanup
	CleanupOld(maxAge time.Duration)
	StartCleanupRoutine(interval, maxAge time.Duration)
}

type service struct {
	llmClient      *Client
	storage        *storage.MinioStorage
	summaryCache   map[string]*SummaryResult
	summaryMu      sync.RWMutex
	chatProcessing map[string]*ChatProcessingResult
	chatMu         sync.RWMutex
	historyCache   map[string]*ChatHistory
	historyMu      sync.RWMutex
	logger         *zap.Logger
}

func New(llmClient *Client, st *storage.MinioStorage) Service {
	return &service{
		llmClient:      llmClient,
		storage:        st,
		summaryCache:   make(map[string]*SummaryResult),
		chatProcessing: make(map[string]*ChatProcessingResult),
		historyCache:   make(map[string]*ChatHistory),
		logger:         logger.L,
	}
}

type SummaryStatus string

const (
	StatusPending    SummaryStatus = "pending"
	StatusProcessing SummaryStatus = "processing"
	StatusCompleted  SummaryStatus = "completed"
	StatusFailed     SummaryStatus = "failed"
	StatusNotFound   SummaryStatus = "not_found"
)

type SummaryResult struct {
	FileID    string        `json:"file_id"`
	Summary   string        `json:"summary,omitempty"`
	Status    SummaryStatus `json:"status"`
	Error     string        `json:"error,omitempty"`
	UpdatedAt time.Time     `json:"updated_at"`
	CreatedAt time.Time     `json:"created_at"`
}

type ChatProcessingStatus string

const (
	ChatStatusNotStarted ChatProcessingStatus = "not_started"
	ChatStatusIndexing   ChatProcessingStatus = "indexing"
	ChatStatusReady      ChatProcessingStatus = "ready"
	ChatStatusFailed     ChatProcessingStatus = "failed"
)

type ChatProcessingResult struct {
	FileID    string               `json:"file_id"`
	Status    ChatProcessingStatus `json:"status"`
	Error     string               `json:"error,omitempty"`
	Message   string               `json:"message,omitempty"`
	UpdatedAt time.Time            `json:"updated_at"`
	CreatedAt time.Time            `json:"created_at"`
}

type ChatResponse struct {
	Answer    string    `json:"answer"`
	FileID    string    `json:"file_id"`
	Question  string    `json:"question"`
	Timestamp time.Time `json:"timestamp"`
}

type ChatMessage struct {
	Question  string    `json:"question"`
	Answer    string    `json:"answer"`
	Timestamp time.Time `json:"timestamp"`
}

type ChatHistory struct {
	FileID    string        `json:"file_id"`
	Messages  []ChatMessage `json:"messages"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

var (
	ErrDocumentNotReady = errors.New("document not ready for chat")
)
