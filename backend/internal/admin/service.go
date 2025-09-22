package admin

import (
	"backend/internal/worker"
	"context"
	"database/sql"
	"errors"
)

func (s *service) Usage(ctx context.Context) ([]UsageRow, error) {
	rows, err := s.db.QueryContext(ctx, `
        SELECT u.id, u.email, COALESCE(SUM(uf.original_size_bytes),0) AS original_bytes
        FROM users u
        LEFT JOIN user_files uf ON uf.user_id = u.id AND uf.deleted_at IS NULL
        GROUP BY u.id, u.email
        ORDER BY original_bytes DESC
        LIMIT 1000
    `)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []UsageRow{}
	for rows.Next() {
		var r UsageRow
		if err := rows.Scan(&r.UserID, &r.Email, &r.OriginalBytes); err != nil {
			return nil, err
		}
		if err := s.db.QueryRowContext(ctx, `
            SELECT COALESCE(SUM(fc.size_bytes),0)
            FROM file_contents fc
            JOIN (
                SELECT DISTINCT content_id FROM user_files
                WHERE user_id=$1 AND deleted_at IS NULL
            ) u ON u.content_id = fc.id
        `, r.UserID).Scan(&r.DedupedBytes); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

func (s *service) Audit(ctx context.Context, limit, offset int) ([]AuditItem, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := s.db.QueryContext(ctx, `
        SELECT id, user_id, action, target_type, target_id, meta, created_at
        FROM audit_logs
        ORDER BY created_at DESC
        LIMIT $1 OFFSET $2
    `, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []AuditItem{}
	for rows.Next() {
		var id string
		var userID, action, targetType, targetID, meta sql.NullString
		var createdAt sql.NullTime
		if err := rows.Scan(&id, &userID, &action, &targetType, &targetID, &meta, &createdAt); err != nil {
			return nil, err
		}
		itm := AuditItem{ID: id}
		if userID.Valid {
			v := userID.String
			itm.UserID = &v
		}
		if action.Valid {
			v := action.String
			itm.Action = &v
		}
		if targetType.Valid {
			v := targetType.String
			itm.TargetType = &v
		}
		if targetID.Valid {
			v := targetID.String
			itm.TargetID = &v
		}
		if meta.Valid {
			v := meta.String
			itm.Meta = &v
		}
		if createdAt.Valid {
			itm.CreatedAt = createdAt.Time
		}
		out = append(out, itm)
	}
	return out, nil
}

func (s *service) ForceDelete(ctx context.Context, actorUserID string, userFileID, contentID string) error {
	if userFileID == "" && contentID == "" {
		return errors.New("userFileId or contentId required")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if userFileID != "" {
		var fcID string
		if err := tx.QueryRowContext(ctx, "SELECT content_id FROM user_files WHERE id=$1", userFileID).Scan(&fcID); err != nil {
			if err == sql.ErrNoRows {
				return err
			}
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM user_files WHERE id=$1", userFileID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE file_contents SET ref_count = GREATEST(ref_count - 1,0) WHERE id=$1", fcID); err != nil {
			return err
		}

		var refCount int64
		if err := tx.QueryRowContext(ctx, "SELECT ref_count FROM file_contents WHERE id=$1", fcID).Scan(&refCount); err == nil && refCount == 0 {
			var blobKey string
			if err := tx.QueryRowContext(ctx, "SELECT blob_key FROM file_contents WHERE id=$1", fcID).Scan(&blobKey); err == nil {
				_, _ = tx.ExecContext(ctx, "DELETE FROM file_contents WHERE id=$1", fcID)
				if s.producer != nil {
					_ = s.producer.PublishGCJob(ctx, worker.GCJob{ContentID: fcID, BlobKey: blobKey})
				}
				_, _ = tx.ExecContext(ctx, "INSERT INTO audit_logs (user_id, action, target_type, target_id, meta, created_at) VALUES ($1,'force_delete','content',$2,$3,now())", actorUserID, fcID, mapToJSON(map[string]string{"blobKey": blobKey}))
			}
		}
	} else if contentID != "" {
		if _, err := tx.ExecContext(ctx, "DELETE FROM user_files WHERE content_id=$1", contentID); err != nil {
			return err
		}
		var blobKey string
		if err := tx.QueryRowContext(ctx, "SELECT blob_key FROM file_contents WHERE id=$1", contentID).Scan(&blobKey); err == nil {
			_, _ = tx.ExecContext(ctx, "DELETE FROM file_contents WHERE id=$1", contentID)
			if s.producer != nil {
				_ = s.producer.PublishGCJob(ctx, worker.GCJob{ContentID: contentID, BlobKey: blobKey})
			}
			_, _ = tx.ExecContext(ctx, "INSERT INTO audit_logs (user_id, action, target_type, target_id, meta, created_at) VALUES ($1,'force_delete','content',$2,$3,now())", actorUserID, contentID, mapToJSON(map[string]string{"blobKey": blobKey}))
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}
