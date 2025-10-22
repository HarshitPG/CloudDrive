package admin

const (
	// Usage aggregation queries
	qGetUserUsageOriginal = `
        SELECT u.id, u.email, COALESCE(SUM(uf.original_size_bytes),0) AS original_bytes
        FROM users u
        LEFT JOIN user_files uf ON uf.user_id = u.id AND uf.deleted_at IS NULL
        GROUP BY u.id, u.email
        ORDER BY original_bytes DESC
        LIMIT 1000
    `

	qGetUserDedupedBytes = `
            SELECT COALESCE(SUM(fc.size_bytes),0)
            FROM file_contents fc
            JOIN (
                SELECT DISTINCT content_id FROM user_files
                WHERE user_id=$1 AND deleted_at IS NULL
            ) u ON u.content_id = fc.id
        `

	// Audit log queries
	qGetAuditLogs = `
        SELECT id, user_id, action, target_type, target_id, meta, created_at
        FROM audit_logs
        ORDER BY created_at DESC
        LIMIT $1 OFFSET $2
    `

	// Force delete queries - user_files operations
	qGetContentIDFromUserFile = `SELECT content_id FROM user_files WHERE id=$1`
	qDeleteUserFileByID       = `DELETE FROM user_files WHERE id=$1`
	qDeleteUserFilesByContent = `DELETE FROM user_files WHERE content_id=$1`

	// Force delete queries - file_contents operations
	qDecRefCountByID       = `UPDATE file_contents SET ref_count = GREATEST(ref_count - 1,0) WHERE id=$1`
	qGetRefCountByID       = `SELECT ref_count FROM file_contents WHERE id=$1`
	qGetBlobKeyByContentID = `SELECT blob_key FROM file_contents WHERE id=$1`
	qDeleteFileContentByID = `DELETE FROM file_contents WHERE id=$1`

	// Audit log insertion
	qInsertAuditLog = `INSERT INTO audit_logs (user_id, action, target_type, target_id, meta, created_at) VALUES ($1,'force_delete','content',$2,$3,now())`
)
