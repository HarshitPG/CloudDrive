package quota

import (
	"context"
)

func (s *service) GetUsage(ctx context.Context, userID string) (Usage, error) {
	var out Usage
	if err := s.db.QueryRowContext(ctx, qGetOriginalBytes, userID).Scan(&out.OriginalBytes); err != nil {
		return Usage{}, err
	}

	if err := s.db.QueryRowContext(ctx, qGetDedupedBytes, userID).Scan(&out.DedupedBytes); err != nil {
		return Usage{}, err
	}

	if err := s.db.QueryRowContext(ctx, qGetQuotaBytes, userID).Scan(&out.QuotaBytes); err != nil {
		return Usage{}, err
	}

	out.SavingsBytes = out.OriginalBytes - out.DedupedBytes
	if out.OriginalBytes > 0 {
		out.SavingsPercent = (float64(out.SavingsBytes) / float64(out.OriginalBytes)) * 100
		if out.SavingsPercent < 0 {
			out.SavingsPercent = 0
		}
	}
	if out.QuotaBytes > 0 {
		out.QuotaUsedPercent = (float64(out.DedupedBytes) / float64(out.QuotaBytes)) * 100
	}

	return out, nil
}

func (s *service) UpdateQuota(ctx context.Context, targetUserID string, quotaBytes int64) error {
	_, err := s.db.ExecContext(ctx, qUpdateQuota, quotaBytes, targetUserID)
	return err
}
