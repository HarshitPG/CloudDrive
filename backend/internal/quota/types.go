package quota

import (
	"context"
	"database/sql"
)

// Service defines quota-related operations.
type Service interface {
	// GetUsage computes usage and quota metrics for a given user.
	GetUsage(ctx context.Context, userID string) (Usage, error)
	// UpdateQuota updates a user's quota in bytes.
	UpdateQuota(ctx context.Context, targetUserID string, quotaBytes int64) error
}

// service is the concrete implementation.
type service struct {
	db *sql.DB
}

// New constructs the quota service.
func New(db *sql.DB) Service { return &service{db: db} }

// Usage captures quota and usage metrics for a user.
type Usage struct {
	OriginalBytes    int64
	DedupedBytes     int64
	QuotaBytes       int64
	SavingsBytes     int64
	SavingsPercent   float64
	QuotaUsedPercent float64
}
