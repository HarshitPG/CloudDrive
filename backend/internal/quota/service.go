package quota

import (
	"context"
)

func (s *service) GetUsage(ctx context.Context, userID string) (Usage, error) {
	var out Usage
	if err := s.db.QueryRowContext(ctx, `
        SELECT COALESCE(SUM(original_size_bytes),0) FROM user_files
        WHERE user_id=$1
    `, userID).Scan(&out.OriginalBytes); err != nil {
		return Usage{}, err
	}

	if err := s.db.QueryRowContext(ctx, `
        SELECT COALESCE(SUM(fc.size_bytes),0)
        FROM file_contents fc
        JOIN (
            SELECT DISTINCT content_id FROM user_files
            WHERE user_id=$1
        ) u ON u.content_id = fc.id
    `, userID).Scan(&out.DedupedBytes); err != nil {
		return Usage{}, err
	}

	if err := s.db.QueryRowContext(ctx, `
        SELECT quota_bytes FROM users WHERE id=$1
    `, userID).Scan(&out.QuotaBytes); err != nil {
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
	_, err := s.db.ExecContext(ctx, `UPDATE users SET quota_bytes=$1 WHERE id=$2`, quotaBytes, targetUserID)
	return err
}
