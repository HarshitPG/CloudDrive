package quota

const (
	// Get total original size of all user files
	qGetOriginalBytes = `
        SELECT COALESCE(SUM(original_size_bytes),0) FROM user_files
        WHERE user_id=$1`

	// Get total deduplicated size (actual storage used)
	qGetDedupedBytes = `
        SELECT COALESCE(SUM(fc.size_bytes),0)
        FROM file_contents fc
        JOIN (
            SELECT DISTINCT content_id FROM user_files
            WHERE user_id=$1
        ) u ON u.content_id = fc.id`

	// Get user's quota limit
	qGetQuotaBytes = `SELECT quota_bytes FROM users WHERE id=$1`

	// Update user's quota
	qUpdateQuota = `UPDATE users SET quota_bytes=$1 WHERE id=$2`
)
