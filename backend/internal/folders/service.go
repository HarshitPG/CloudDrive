package folders

import (
	"archive/zip"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"

	"backend/internal/cache"
	"backend/internal/worker"
	"backend/pkg/logger"

	"go.uber.org/zap"
)

func (s *service) ListPrimary(ctx context.Context, userID, parentID string, deleted bool, limit, offset int) (PagedFolders, error) {
	limit, offset = normalizeLimitOffset(limit, offset)
	var (
		rows *sql.Rows
		err  error
	)
	if deleted {
		rows, err = s.db.QueryContext(ctx, qListTrashPrimary, userID, limit, offset)
	} else if parentID == "" {
		rows, err = s.db.QueryContext(ctx, qListRootPrimary, userID, limit, offset)
	} else {
		rows, err = s.db.QueryContext(ctx, qListChildrenPrimary, userID, parentID, limit, offset)
	}
	if err != nil {
		return PagedFolders{}, err
	}
	defer rows.Close()

	out := make([]FolderItem, 0, limit)
	for rows.Next() {
		var it FolderItem
		var deletedAt sql.NullTime
		var size sql.NullInt64
		if deleted {
			if err := rows.Scan(&it.ID, &it.Name, &it.CreatedAt, &it.UpdatedAt, &deletedAt, &size); err != nil {
				continue
			}
			if deletedAt.Valid {
				t := deletedAt.Time.Format(timeLayout)
				it.DeletedAt = &t
			}
		} else {
			if err := rows.Scan(&it.ID, &it.Name, &it.CreatedAt, &it.UpdatedAt, &size); err != nil {
				continue
			}
		}
		if size.Valid {
			it.Size = size.Int64
		}
		out = append(out, it)
	}
	return PagedFolders{Folders: out, Limit: limit, Offset: offset}, nil
}

func (s *service) Create(ctx context.Context, userID, parentID, name string) (string, error) {
	var id string
	if err := s.db.QueryRowContext(ctx, qCreateFolder, userID, parentID, name).Scan(&id); err != nil {
		return "", err
	}
	if parentID != "" {
		cache.InvalidateFolder(ctx, s.cache, parentID)
	}
	return id, nil
}

func (s *service) ListContents(ctx context.Context, userID, folderID string) (FolderContents, error) {
	var owner string
	if err := s.db.QueryRowContext(ctx, qValidateFolderOwner, folderID).Scan(&owner); err != nil {
		return FolderContents{}, err
	}
	if owner != userID {
		return FolderContents{}, errors.New("forbidden")
	}

	key := folderContentsKey(folderID)
	if s.cache != nil {
		var cached FolderContents
		if err := s.cache.Get(ctx, key, &cached); err == nil {
			return cached, nil
		}
	}

	srows, err := s.db.QueryContext(ctx, qListSubfolders, userID, folderID)
	if err != nil {
		return FolderContents{}, err
	}
	defer srows.Close()
	subs := []FolderChild{}
	for srows.Next() {
		var it FolderChild
		if err := srows.Scan(&it.ID, &it.Name, &it.CreatedAt); err != nil {
			continue
		}
		subs = append(subs, it)
	}

	frows, err := s.db.QueryContext(ctx, qListFolderFiles, userID, folderID)
	if err != nil {
		return FolderContents{}, err
	}
	defer frows.Close()
	files := []FileChild{}
	for frows.Next() {
		var it FileChild
		if err := frows.Scan(&it.ID, &it.Filename, &it.MIME, &it.Size, &it.CreatedAt); err != nil {
			continue
		}
		files = append(files, it)
	}

	resp := FolderContents{Folders: subs, Files: files}
	if s.cache != nil {
		_ = s.cache.Set(ctx, key, resp, folderListTTL)
	}
	return resp, nil
}

func (s *service) Rename(ctx context.Context, userID, folderID, name string) error {
	if _, err := s.db.ExecContext(ctx, qRenameFolder, name, folderID, userID); err != nil {
		return err
	}
	cache.InvalidateFolder(ctx, s.cache, folderID)
	return nil
}

func (s *service) WriteArchive(ctx context.Context, userID, folderID string, recursive bool, w io.Writer) error {
	var owner string
	if err := s.db.QueryRowContext(ctx, qValidateFolderOwner, folderID).Scan(&owner); err != nil {
		return err
	}
	if owner != userID {
		return errors.New("forbidden")
	}
	if recursive {
		rows, qerr := s.db.QueryContext(ctx, `
            WITH RECURSIVE subfolders AS (
                SELECT id, name, parent_id, name AS path
                FROM folders WHERE id=$1 AND user_id=$2
                UNION ALL
                SELECT f.id, f.name, f.parent_id, sub.path || '/' || f.name AS path
                FROM folders f
                JOIN subfolders sub ON f.parent_id = sub.id
                WHERE f.user_id=$2
            )
            SELECT uf.id,
                   CASE WHEN sub.path IS NULL OR sub.path = '' THEN uf.filename
                        ELSE sub.path || '/' || uf.filename END AS entry_path
            FROM user_files uf
            JOIN subfolders sub ON uf.folder_id = sub.id
            WHERE uf.user_id=$2 AND uf.deleted_at IS NULL
        `, folderID, userID)
		if qerr != nil {
			return qerr
		}
		defer rows.Close()

		zipWriter := zip.NewWriter(w)
		defer zipWriter.Close()

		hasAny := false
		for rows.Next() {
			var fid, entryPath string
			if err := rows.Scan(&fid, &entryPath); err != nil {
				continue
			}
			hasAny = true
			if err := s.addFileToZipByIDWithPath(ctx, zipWriter, fid, entryPath); err != nil {
				logger.L.Warn("folders.archive: add file failed", zap.String("fileId", fid), zap.Error(err))
				continue
			}
		}
		if !hasAny {
			return sql.ErrNoRows
		}
	} else {
		rows, qerr := s.db.QueryContext(ctx, `
            SELECT uf.id
            FROM user_files uf
            WHERE uf.folder_id = $1 AND uf.user_id=$2 AND uf.deleted_at IS NULL
        `, folderID, userID)
		if qerr != nil {
			return qerr
		}
		defer rows.Close()

		zipWriter := zip.NewWriter(w)
		defer zipWriter.Close()

		hasAny := false
		for rows.Next() {
			var fid string
			if err := rows.Scan(&fid); err != nil {
				continue
			}
			hasAny = true
			if err := s.addFileToZipByID(ctx, zipWriter, fid); err != nil {
				logger.L.Warn("folders.archive: add file failed", zap.String("fileId", fid), zap.Error(err))
				continue
			}
		}
		if !hasAny {
			return sql.ErrNoRows
		}
	}
	return nil
}

func (s *service) Move(ctx context.Context, userID, folderID, targetParentID string) error {
	if folderID == targetParentID {
		return fmt.Errorf("cannot move folder inside itself")
	}
	if _, err := s.db.ExecContext(ctx, qMoveFolder, targetParentID, folderID, userID); err != nil {
		return err
	}
	cache.InvalidateFolder(ctx, s.cache, folderID)
	if targetParentID != "" {
		cache.InvalidateFolder(ctx, s.cache, targetParentID)
	}
	return nil
}

func (s *service) ListFilesInFolder(ctx context.Context, userID, folderID string, limit, offset int) (PagedFiles, error) {
	limit, offset = normalizeLimitOffset(limit, offset)

	var owner string
	if err := s.db.QueryRowContext(ctx, qValidateFolderOwner, folderID).Scan(&owner); err != nil {
		return PagedFiles{}, err
	}
	if owner != userID {
		return PagedFiles{}, errors.New("forbidden")
	}

	rows, err := s.db.QueryContext(ctx, qListFilesInFolder, userID, folderID, limit, offset)
	if err != nil {
		return PagedFiles{}, err
	}
	defer rows.Close()

	out := make([]FileListItem, 0, limit)
	for rows.Next() {
		var it FileListItem
		if err := rows.Scan(&it.ID, &it.Filename, &it.MIME, &it.Size, &it.CreatedAt, &it.UpdatedAt, &it.DownloadCount, &it.ContentHash, &it.PhysicalSize, &it.RefCount); err != nil {
			continue
		}
		it.DedupSavings = it.Size - it.PhysicalSize
		out = append(out, it)
	}
	return PagedFiles{Files: out, Limit: limit, Offset: offset}, nil
}

func (s *service) GetTree(ctx context.Context, userID, rootID string) (FolderTree, error) {
	var name, createdAt string
	if err := s.db.QueryRowContext(ctx, qTreeFolder, rootID, userID).Scan(&name, &createdAt); err != nil {
		return FolderTree{}, err
	}
	rows, err := s.db.QueryContext(ctx, qTreeChildren, rootID, userID)
	if err != nil {
		return FolderTree{}, err
	}
	defer rows.Close()
	kids := []FolderTree{}
	for rows.Next() {
		var childID string
		if err := rows.Scan(&childID); err == nil {
			child, err := s.GetTree(ctx, userID, childID)
			if err == nil {
				kids = append(kids, child)
			}
		}
	}
	return FolderTree{ID: rootID, Name: name, CreatedAt: createdAt, Children: kids}, nil
}

func (s *service) GetAncestors(ctx context.Context, userID, folderID string) ([]Ancestor, error) {
	rows, err := s.db.QueryContext(ctx, qAncestors, folderID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type anc struct{ id, name string }
	list := make([]anc, 0)
	for rows.Next() {
		var a anc
		if err := rows.Scan(&a.id, &a.name); err == nil {
			list = append(list, a)
		}
	}
	for i, j := 0, len(list)-1; i < j; i, j = i+1, j-1 {
		list[i], list[j] = list[j], list[i]
	}
	out := make([]Ancestor, 0, len(list))
	for _, a := range list {
		out = append(out, Ancestor{ID: a.id, Name: a.name})
	}
	return out, nil
}

func (s *service) Delete(ctx context.Context, userID, folderID string, permanent bool) (string, error) {
	if !permanent {
		return s.softDelete(ctx, userID, folderID)
	}
	return s.hardDelete(ctx, userID, folderID)
}

func (s *service) softDelete(ctx context.Context, userID, folderID string) (string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}

	type fileRow struct{ fileID, contentID, blobKey string }
	files := make([]fileRow, 0, 64)
	frows, err := tx.QueryContext(ctx, qSoftDeleteSubtreeFiles, folderID, userID)
	if err != nil {
		_ = tx.Rollback()
		return "", err
	}
	for frows.Next() {
		var fr fileRow
		if err := frows.Scan(&fr.fileID, &fr.contentID, &fr.blobKey); err != nil {
			_ = frows.Close()
			_ = tx.Rollback()
			return "", err
		}
		files = append(files, fr)
	}
	_ = frows.Close()
	for _, fr := range files {
		if _, err := tx.ExecContext(ctx, qMarkFileDeleted, fr.fileID); err != nil {
			_ = tx.Rollback()
			return "", err
		}
		if _, err := tx.ExecContext(ctx, qDecrementRef, fr.contentID); err != nil {
			_ = tx.Rollback()
			return "", err
		}
		if _, err := tx.ExecContext(ctx, qRevokeFileShare, fr.fileID); err != nil {
			logger.L.Warn("folders.softdelete: revoke file shares failed", zap.Error(err), zap.String("fileID", fr.fileID))
		}
	}
	if _, err := tx.ExecContext(ctx, qRevokeFolderShares, folderID, userID); err != nil {
		logger.L.Warn("folders.softdelete: revoke folder shares failed", zap.Error(err), zap.String("folderID", folderID))
	}
	if _, err := tx.ExecContext(ctx, qMarkFoldersDeleted, folderID, userID); err != nil {
		_ = tx.Rollback()
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	cache.InvalidateFolder(ctx, s.cache, folderID)
	cache.InvalidateSearch(ctx, s.cache, userID)
	return "folder moved to trash (subtree) and shares revoked", nil
}

func (s *service) hardDelete(ctx context.Context, userID, folderID string) (string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	type fileRow struct{ fileID, contentID, blobKey string }
	files := make([]fileRow, 0, 64)
	frows, err := tx.QueryContext(ctx, qListAllSubtreeFiles, folderID, userID)
	if err != nil {
		_ = tx.Rollback()
		return "", err
	}
	for frows.Next() {
		var fr fileRow
		if err := frows.Scan(&fr.fileID, &fr.contentID, &fr.blobKey); err != nil {
			_ = frows.Close()
			_ = tx.Rollback()
			return "", err
		}
		files = append(files, fr)
	}
	_ = frows.Close()

	for _, fr := range files {
		if _, err := tx.ExecContext(ctx, qDeleteUserFile, fr.fileID); err != nil {
			_ = tx.Rollback()
			return "", err
		}
		var refCount int64
		if err := tx.QueryRowContext(ctx, qDecRefReturn, fr.contentID).Scan(&refCount); err != nil {
			_ = tx.Rollback()
			return "", err
		}
		if refCount == 0 {
			var remaining int64
			if err := tx.QueryRowContext(ctx, qCountRefs, fr.contentID).Scan(&remaining); err != nil {
				_ = tx.Rollback()
				return "", err
			}
			if remaining == 0 {
				if s.producer != nil {
					_ = s.producer.PublishGCJob(ctx, worker.GCJob{ContentID: fr.contentID, BlobKey: fr.blobKey})
				}
				if _, err := tx.ExecContext(ctx, qDeleteContent, fr.contentID); err != nil {
					_ = tx.Rollback()
					return "", err
				}
			}
		}
		if _, err := tx.ExecContext(ctx, qDeleteFileShares, fr.fileID); err != nil {
			logger.L.Warn("folders.harddelete: delete file shares failed", zap.Error(err), zap.String("fileID", fr.fileID))
		}
	}
	if _, err := tx.ExecContext(ctx, qDeleteFolderShares, folderID, userID); err != nil {
		logger.L.Warn("folders.harddelete: delete folder shares failed", zap.Error(err), zap.String("folderID", folderID))
	}
	if _, err := tx.ExecContext(ctx, qDeleteFolders, folderID, userID); err != nil {
		_ = tx.Rollback()
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	cache.InvalidateFolder(ctx, s.cache, folderID)
	cache.InvalidateSearch(ctx, s.cache, userID)
	return "folder permanently deleted", nil
}

const timeLayout = "2006-01-02T15:04:05Z07:00"

func (s *service) addFileToZipByID(ctx context.Context, zipWriter *zip.Writer, fileID string) error {
	var blobKey, filename string
	err := s.db.QueryRowContext(ctx, `
        SELECT fc.blob_key, uf.filename
        FROM user_files uf
        JOIN file_contents fc ON uf.content_id = fc.id
        WHERE uf.id = $1 AND uf.deleted_at IS NULL
    `, fileID).Scan(&blobKey, &filename)
	if err != nil {
		return err
	}

	url, err := s.storage.PresignedGetURL(ctx, blobKey, 5)
	if err != nil {
		return err
	}
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: %d", resp.StatusCode)
	}

	zipFile, err := zipWriter.Create(filename)
	if err != nil {
		zipFile, err = zipWriter.Create(fmt.Sprintf("%s-%s", filename, fileID))
		if err != nil {
			return err
		}
	}
	_, err = io.Copy(zipFile, resp.Body)
	return err
}

func (s *service) addFileToZipByIDWithPath(ctx context.Context, zipWriter *zip.Writer, fileID string, entryPath string) error {
	var blobKey string
	err := s.db.QueryRowContext(ctx, `
        SELECT fc.blob_key
        FROM user_files uf
        JOIN file_contents fc ON uf.content_id = fc.id
        WHERE uf.id = $1 AND uf.deleted_at IS NULL
    `, fileID).Scan(&blobKey)
	if err != nil {
		return err
	}

	url, err := s.storage.PresignedGetURL(ctx, blobKey, 5)
	if err != nil {
		return err
	}
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: %d", resp.StatusCode)
	}

	zipFile, err := zipWriter.Create(entryPath)
	if err != nil {
		return err
	}
	_, err = io.Copy(zipFile, resp.Body)
	return err
}
