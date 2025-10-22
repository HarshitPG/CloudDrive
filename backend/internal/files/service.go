package files

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	iCache "backend/internal/cache"
	"backend/internal/storage"
	"backend/internal/worker"
	perr "backend/pkg/errors"
	"backend/pkg/logger"

	"go.uber.org/zap"
)

func (s *service) GetDownloadURL(ctx context.Context, userID, fileID string) (string, error) {
	if userID == "" {
		return "", perr.ErrUnauthorized
	}
	var contentID, blobKey, owner string
	if err := s.db.QueryRowContext(ctx, qSelectForDownload, fileID).Scan(&contentID, &blobKey, &owner); err != nil {
		if err == sql.ErrNoRows {
			return "", perr.ErrNotFound
		}
		return "", err
	}
	allowed := owner == userID
	if !allowed {
		var count int
		if err := s.db.QueryRowContext(ctx, qCheckShareAccess, fileID, userID).Scan(&count); err == nil && count > 0 {
			allowed = true
		}
	}
	if !allowed {
		return "", perr.ErrUnauthorized
	}
	if s.storage == nil {
		return "", fmt.Errorf("storage not configured")
	}
	url, err := s.storage.PresignedGetURL(ctx, blobKey, 5)
	if err != nil {
		return "", err
	}
	var newCount int64
	if err := s.db.QueryRowContext(ctx, qIncDownload, fileID).Scan(&newCount); err != nil {
		logger.L.Warn("failed inc download_count", zap.Error(err))
	} else if s.publish != nil {
		_ = s.publish(ctx, fileID, newCount)
	}
	return url, nil
}

func (s *service) Delete(ctx context.Context, userID, fileID string, permanent bool) (string, error) {
	var owner, contentID, blobKey string
	var folderId sql.NullString
	cond := "uf.deleted_at IS NULL"
	if permanent {
		cond = "TRUE"
	}
	if err := s.db.QueryRowContext(ctx, qSelectOwnerContentFolder+cond, fileID).Scan(&owner, &contentID, &blobKey, &folderId); err != nil {
		if err == sql.ErrNoRows {
			return "", perr.ErrNotFound
		}
		return "", err
	}
	if owner != userID {
		return "", perr.ErrUnauthorized
	}

	if !permanent {
		tx, _ := s.db.BeginTx(ctx, nil)
		if _, err := tx.ExecContext(ctx, qSoftDeleteFile, fileID); err != nil {
			_ = tx.Rollback()
			return "", err
		}
		if _, err := tx.ExecContext(ctx, qDecRefCount, contentID); err != nil {
			_ = tx.Rollback()
			return "", err
		}
		_, _ = tx.ExecContext(ctx, qRevokeShares, fileID)
		if err := tx.Commit(); err != nil {
			return "", err
		}
		if s.cache != nil {
			if folderId.Valid {
				_ = s.cache.Delete(ctx, iCache.FolderContentsKey(folderId.String))
			} else {
				var f sql.NullString
				_ = s.db.QueryRowContext(ctx, qSelectFolderForFile, fileID).Scan(&f)
				if f.Valid {
					_ = s.cache.Delete(ctx, iCache.FolderContentsKey(f.String))
				}
			}
			_ = s.cache.Delete(ctx, iCache.FileMetadataKey(fileID))
		}
		return "deleted and shares revoked", nil
	}

	tx, _ := s.db.BeginTx(ctx, nil)
	if _, err := tx.ExecContext(ctx, qDeleteUserFile, fileID); err != nil {
		_ = tx.Rollback()
		return "", err
	}
	var refCount int64
	if err := tx.QueryRowContext(ctx, qDecRefReturn, contentID).Scan(&refCount); err != nil {
		_ = tx.Rollback()
		return "", err
	}

	if refCount == 0 {
		var remaining int
		if err := tx.QueryRowContext(ctx, qCountUserFilesByContentID, contentID).Scan(&remaining); err != nil {
			logger.L.Error("count remaining user_files failed", zap.Error(err), zap.String("contentID", contentID))
		}
		if remaining == 0 {
			if s.storage != nil && s.storage.Client != nil {
				if err := s.storage.Client.RemoveObject(ctx, s.storage.Bucket, blobKey, storage.MinioRemoveOpts()); err != nil {
					if s.producer != nil {
						_ = s.producer.PublishGCJob(ctx, worker.GCJob{ContentID: contentID, BlobKey: blobKey})
					}
				}
			} else if s.producer != nil {
				_ = s.producer.PublishGCJob(ctx, worker.GCJob{ContentID: contentID, BlobKey: blobKey})
			}
			if _, err := tx.ExecContext(ctx, qDeleteFileContent, contentID); err != nil {
				logger.L.Error("delete file_contents failed", zap.Error(err), zap.String("contentID", contentID))
			}
		}
	}
	_, _ = tx.ExecContext(ctx, qDeleteSharesPermanent, fileID)
	if err := tx.Commit(); err != nil {
		return "", err
	}

	if s.cache != nil {
		if folderId.Valid {
			_ = s.cache.Delete(ctx, iCache.FolderContentsKey(folderId.String))
		}
		_ = s.cache.Delete(ctx, iCache.FileMetadataKey(fileID))
	}
	return "permanently deleted and shares revoked", nil
}

func (s *service) Restore(ctx context.Context, userID, fileID string) (string, error) {
	var owner, contentID string
	if err := s.db.QueryRowContext(ctx, qCheckSoftDeleted, fileID).Scan(&owner, &contentID); err != nil {
		if err == sql.ErrNoRows {
			return "", perr.ErrNotFound
		}
		return "", err
	}
	if owner != userID {
		return "", perr.ErrUnauthorized
	}
	tx, _ := s.db.BeginTx(ctx, nil)
	if _, err := tx.ExecContext(ctx, qRestoreFile, fileID); err != nil {
		_ = tx.Rollback()
		return "", err
	}
	if _, err := tx.ExecContext(ctx, qIncRefCount, contentID); err != nil {
		_ = tx.Rollback()
		return "", err
	}
	_, _ = tx.ExecContext(ctx, qEnableShares, fileID)
	if err := tx.Commit(); err != nil {
		return "", err
	}
	if s.cache != nil {
		var folderId sql.NullString
		_ = s.db.QueryRowContext(ctx, qSelectFolderForFile, fileID).Scan(&folderId)
		if folderId.Valid {
			_ = s.cache.Delete(ctx, iCache.FolderContentsKey(folderId.String))
		}
		_ = s.cache.Delete(ctx, iCache.FileMetadataKey(fileID))
	}
	return "restored (shares re-enabled)", nil
}

func (s *service) Patch(ctx context.Context, userID, fileID string, req PatchRequest) error {
	var owner string
	if err := s.db.QueryRowContext(ctx, qSelectOwnerActive, fileID).Scan(&owner); err != nil {
		if err == sql.ErrNoRows {
			return perr.ErrNotFound
		}
		return err
	}
	if owner != userID {
		return perr.ErrUnauthorized
	}
	if req.Filename != "" {
		if _, err := s.db.ExecContext(ctx, qUpdateFilename, req.Filename, fileID); err != nil {
			return err
		}
	}
	if req.Tags != nil {
		b, _ := json.Marshal(req.Tags)
		if _, err := s.db.ExecContext(ctx, qUpdateTags, b, fileID); err != nil {
			return err
		}
	}
	if s.cache != nil {
		_ = s.cache.Delete(ctx, iCache.FileMetadataKey(fileID))
	}
	return nil
}

func (s *service) Move(ctx context.Context, userID, fileID, targetFolderID string) error {
	var owner string
	if err := s.db.QueryRowContext(ctx, qSelectOwnerActive, fileID).Scan(&owner); err != nil {
		if err == sql.ErrNoRows {
			return perr.ErrNotFound
		}
		return err
	}
	if owner != userID {
		return perr.ErrUnauthorized
	}
	var folderOwner string
	if err := s.db.QueryRowContext(ctx, qSelectFolderOwner, targetFolderID).Scan(&folderOwner); err != nil {
		return perr.ErrNotFound
	}
	if folderOwner != userID {
		return perr.ErrUnauthorized
	}
	var originalFolderId sql.NullString
	_ = s.db.QueryRowContext(ctx, qSelectOriginalFolder, fileID).Scan(&originalFolderId)
	if _, err := s.db.ExecContext(ctx, qUpdateMove, targetFolderID, fileID); err != nil {
		return err
	}
	if s.cache != nil {
		if originalFolderId.Valid {
			_ = s.cache.Delete(ctx, iCache.FolderContentsKey(originalFolderId.String))
		}
		_ = s.cache.Delete(ctx, iCache.FolderContentsKey(targetFolderID))
		_ = s.cache.Delete(ctx, iCache.FileMetadataKey(fileID))
	}
	return nil
}

func (s *service) CreateVersion(ctx context.Context, userID, fileID string, payload CreateVersionRequest) error {
	var owner string
	if err := s.db.QueryRowContext(ctx, qSelectOwnerAny, fileID).Scan(&owner); err != nil {
		return perr.ErrNotFound
	}
	if owner != userID {
		return perr.ErrUnauthorized
	}
	if _, err := s.db.ExecContext(ctx, qInsertVersion, userID, payload.ContentID, payload.Filename, fileID); err != nil {
		return err
	}
	return nil
}

func (s *service) ListVersions(ctx context.Context, fileID string) ([]FileVersion, error) {
	rows, err := s.db.QueryContext(ctx, qListVersions, fileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FileVersion
	for rows.Next() {
		var v FileVersion
		if err := rows.Scan(&v.ID, &v.ContentID, &v.Filename, &v.CreatedAt); err == nil {
			out = append(out, v)
		}
	}
	return out, nil
}

func (s *service) GetMetadata(ctx context.Context, userID, fileID string) (FileMetadata, error) {
	var m FileMetadata
	if s.cache != nil {
		if err := s.cache.Get(ctx, fileMetadataKey(fileID), &m); err == nil {
			return m, nil
		}
	}
	var folderID *string
	if err := s.db.QueryRowContext(ctx, qGetMetadata, fileID, userID).Scan(
		&m.Filename, &m.MIME, &m.Size,
		&m.CreatedAt, &m.UpdatedAt, &m.DownloadCount,
		&folderID,
		&m.ContentHash, &m.PhysicalSize, &m.RefCount,
	); err != nil {
		if err == sql.ErrNoRows {
			return m, perr.ErrNotFound
		}
		return m, err
	}
	m.FolderID = folderID
	m.DedupSavings = m.Size - m.PhysicalSize
	if s.cache != nil {
		_ = s.cache.Set(ctx, fileMetadataKey(fileID), m, fiveMinutes)
	}
	return m, nil
}

func scanFiles(rows *sql.Rows, deleted bool) ([]FileListItem, error) {
	var list []FileListItem
	for rows.Next() {
		var it FileListItem
		var deletedAt sql.NullTime
		if deleted {
			if err := rows.Scan(&it.ID, &it.Filename, &it.MIME, &it.Size,
				&it.CreatedAt, &it.UpdatedAt, &it.DownloadCount,
				&it.ContentHash, &it.PhysicalSize, &it.RefCount,
				&deletedAt); err != nil {
				continue
			}
			if deletedAt.Valid {
				s := deletedAt.Time.Format(time.RFC3339)
				it.DeletedAt = &s
			}
		} else {
			if err := rows.Scan(&it.ID, &it.Filename, &it.MIME, &it.Size,
				&it.CreatedAt, &it.UpdatedAt, &it.DownloadCount,
				&it.ContentHash, &it.PhysicalSize, &it.RefCount); err != nil {
				continue
			}
		}
		it.DedupSavings = it.Size - it.PhysicalSize
		list = append(list, it)
	}
	return list, nil
}

func (s *service) ListFiles(ctx context.Context, userID, folderID string, deleted bool) ([]FileListItem, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if deleted {
		rows, err = s.db.QueryContext(ctx, qListTrash, userID)
	} else if folderID != "" {
		rows, err = s.db.QueryContext(ctx, qListInFolder, userID, folderID)
	} else {
		rows, err = s.db.QueryContext(ctx, qListRoot, userID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFiles(rows, deleted)
}

func (s *service) ListFilesPrimary(ctx context.Context, userID, folderID string, deleted bool) ([]FileListItem, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if deleted {
		rows, err = s.db.QueryContext(ctx, qListTrashPrimary, userID)
	} else if folderID != "" {
		rows, err = s.db.QueryContext(ctx, qListInFolderPrimary, userID, folderID)
	} else {
		rows, err = s.db.QueryContext(ctx, qListRootPrimary, userID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFiles(rows, deleted)
}
