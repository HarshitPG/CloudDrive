package admin

import (
	"context"
	"database/sql"
	"time"

	"backend/internal/worker"
)

type Service interface {
	Usage(ctx context.Context) ([]UsageRow, error)
	Audit(ctx context.Context, limit, offset int) ([]AuditItem, error)
	ForceDelete(ctx context.Context, actorUserID string, userFileID, contentID string) error
}

type service struct {
	db       *sql.DB
	producer *worker.Producer
}

func New(db *sql.DB, producer *worker.Producer) Service {
	return &service{db: db, producer: producer}
}

type UsageRow struct {
	UserID        string `json:"userId"`
	Email         string `json:"email"`
	OriginalBytes int64  `json:"originalBytes"`
	DedupedBytes  int64  `json:"dedupedBytes"`
}

type AuditItem struct {
	ID         string    `json:"id"`
	UserID     *string   `json:"userId,omitempty"`
	Action     *string   `json:"action,omitempty"`
	TargetType *string   `json:"targetType,omitempty"`
	TargetID   *string   `json:"targetId,omitempty"`
	Meta       *string   `json:"meta,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}
