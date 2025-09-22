package share

import (
	"archive/zip"
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"time"

	"backend/internal/auth"
	"backend/internal/cache"
	"backend/internal/worker"
	"backend/pkg/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func (h *Handler) validateFolderOwnership(ctx context.Context, folderID, userID string) (string, error) {
	var folderName, owner string
	err := h.DB.QueryRowContext(ctx,
		"SELECT user_id, name FROM folders WHERE id=$1 AND deleted_at IS NULL",
		folderID).Scan(&owner, &folderName)
	if err != nil {
		return "", err
	}
	if owner != userID {
		return "", fmt.Errorf("unauthorized access to folder")
	}
	return folderName, nil
}

func (h *Handler) createShare(ctx context.Context, token, userID string, req CreateFolderShareReq) (string, time.Time, error) {
	var shareID string
	var createdAt time.Time
	err := h.DB.QueryRowContext(ctx, `
        INSERT INTO shares (id, token, creator_id, target_type, target_id, title, description, 
            expires_at, recursive, snapshot_mode, created_at)
        VALUES (gen_random_uuid(), $1, $2, 'folder', $3, $4, $5, $6, $7, $8, now())
        RETURNING id, created_at
    `, token, userID, req.FolderID, req.Title, req.Description, req.ExpiresAt, req.Recursive, req.SnapshotMode).Scan(&shareID, &createdAt)
	return shareID, createdAt, err
}

func (h *Handler) enqueueSnapshotJob(ctx context.Context, shareID, folderID, token string, createdAt time.Time) {
	job := worker.FolderShareJob{ShareID: shareID, FolderID: folderID, Token: token, Recursive: true, SnapshotMode: true, CreatedAt: createdAt.Format(time.RFC3339)}
	if err := h.Producer.PublishFolderShareJob(ctx, job); err != nil {
		logger.L.Warn("failed to enqueue snapshot job", zap.Error(err))
	}
}

func (h *Handler) publishShareEvent(ctx context.Context, eventType, shareID, folderID, token, userID, ipAddress string, timestamp time.Time) {
	event := worker.FolderShareEvent{Type: eventType, ShareID: shareID, FolderID: folderID, Token: token, UserID: userID, IPAddress: ipAddress, Timestamp: timestamp.Format(time.RFC3339)}
	if err := h.Producer.PublishFolderShareEvent(ctx, event); err != nil {
		logger.L.Warn("failed to publish share event", zap.String("eventType", eventType), zap.Error(err))
	}
}

func (h *Handler) invalidateShareCache(ctx context.Context, token string) {
	if h.Cache != nil {
		cache.InvalidateShare(ctx, h.Cache, token)
	}
}

func (h *Handler) getShareOwnership(ctx context.Context, shareID string) (string, string, error) {
	var creatorID, token string
	err := h.DB.QueryRowContext(ctx,
		"SELECT creator_id, token FROM shares WHERE id=$1", shareID).Scan(&creatorID, &token)
	return creatorID, token, err
}

func (h *Handler) revokeShareInDB(ctx context.Context, shareID string) error {
	_, err := h.DB.ExecContext(ctx, "UPDATE shares SET revoked=true WHERE id=$1", shareID)
	return err
}

func (h *Handler) getShareByID(ctx context.Context, shareID string) (*folderShareRecord, error) {
	var share folderShareRecord
	var expiresAt sql.NullTime
	err := h.DB.QueryRowContext(ctx, `
        SELECT s.id, s.token, s.creator_id, s.target_id, f.name, s.title, s.description,
               s.recursive, s.snapshot_mode, s.expires_at, s.created_at
        FROM shares s
        JOIN folders f ON s.target_id = f.id
        WHERE s.id=$1 AND s.target_type='folder' AND s.revoked=false
    `, shareID).Scan(&share.ID, &share.Token, &share.CreatorID, &share.FolderID, &share.FolderName, &share.Title, &share.Description, &share.Recursive, &share.SnapshotMode, &expiresAt, &share.CreatedAt)
	if err != nil {
		return nil, err
	}
	if expiresAt.Valid {
		share.ExpiresAt = &expiresAt.Time
	}
	return &share, nil
}

func (h *Handler) buildShareResponse(share *folderShareRecord) FolderShareResponse {
	return FolderShareResponse{ID: share.ID, Token: share.Token, FolderID: share.FolderID, FolderName: share.FolderName, Title: share.Title, Description: share.Description, Recursive: share.Recursive, SnapshotMode: share.SnapshotMode, ExpiresAt: share.ExpiresAt, CreatedAt: share.CreatedAt, URL: "/fs/" + share.Token}
}

func (h *Handler) addSubfolders(ctx context.Context, items *[]ShareItem, folderID, userID string) error {
	rows, err := h.DB.QueryContext(ctx, `
        SELECT id, name, parent_id
        FROM folders
        WHERE parent_id = $1 AND user_id = $2 AND deleted_at IS NULL
        ORDER BY name
        LIMIT $3
    `, folderID, userID, maxFoldersPerQuery)
	if err != nil {
		return fmt.Errorf("subfolder query failed: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var item ShareItem
		var parentID string
		if err := rows.Scan(&item.ID, &item.Name, &parentID); err != nil {
			logger.L.Error("failed to scan subfolder", zap.Error(err))
			continue
		}
		item.Type = "folder"
		item.Path = item.Name
		item.ParentID = &parentID
		*items = append(*items, item)
	}
	return rows.Err()
}

func (h *Handler) addFiles(ctx context.Context, items *[]ShareItem, folderID, userID string) error {
	rows, err := h.DB.QueryContext(ctx, `
        SELECT id, filename, original_size_bytes, declared_mime, folder_id
        FROM user_files
        WHERE folder_id = $1 AND user_id = $2 AND deleted_at IS NULL
        ORDER BY filename
        LIMIT $3
    `, folderID, userID, maxFilesPerQuery)
	if err != nil {
		return fmt.Errorf("file query failed: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var item ShareItem
		var size int64
		var parentFolderID string
		if err := rows.Scan(&item.ID, &item.Name, &size, &item.MimeType, &parentFolderID); err != nil {
			logger.L.Error("failed to scan file", zap.Error(err))
			continue
		}
		item.Type = "file"
		item.Size = &size
		item.Path = item.Name
		item.ParentID = &parentFolderID
		*items = append(*items, item)
	}
	return rows.Err()
}

func (h *Handler) addRecursiveFolders(ctx context.Context, items *[]ShareItem, folderID, userID string) error {
	rows, err := h.DB.QueryContext(ctx, `
        WITH RECURSIVE folder_tree AS (
            SELECT id, name, parent_id, name as folder_path
            FROM folders
            WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
            
            UNION ALL
            
            SELECT f.id, f.name, f.parent_id, 
                   CASE 
                       WHEN ft.folder_path = '' THEN f.name
                       ELSE ft.folder_path || '/' || f.name
                   END as folder_path
            FROM folders f
            INNER JOIN folder_tree ft ON f.parent_id = ft.id
            WHERE f.user_id = $2 AND f.deleted_at IS NULL
        )
        SELECT id, name, parent_id, folder_path
        FROM folder_tree
        WHERE id != $1
        ORDER BY folder_path
        LIMIT $3
    `, folderID, userID, maxFoldersPerQuery)
	if err != nil {
		return fmt.Errorf("recursive folder query failed: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var item ShareItem
		var folderPath, parentID string
		if err := rows.Scan(&item.ID, &item.Name, &parentID, &folderPath); err != nil {
			logger.L.Error("failed to scan recursive folder", zap.Error(err))
			continue
		}
		item.Type = "folder"
		item.Path = folderPath
		item.ParentID = &parentID
		*items = append(*items, item)
	}
	return rows.Err()
}

func (h *Handler) addRecursiveFiles(ctx context.Context, items *[]ShareItem, folderID, userID string) error {
	rows, err := h.DB.QueryContext(ctx, `
        WITH RECURSIVE folder_tree AS (
            SELECT id, name, parent_id, name as folder_path
            FROM folders
            WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
            
            UNION ALL
            
            SELECT f.id, f.name, f.parent_id,
                   CASE 
                       WHEN ft.folder_path = '' THEN f.name
                       ELSE ft.folder_path || '/' || f.name
                   END as folder_path
            FROM folders f
            INNER JOIN folder_tree ft ON f.parent_id = ft.id
            WHERE f.user_id = $2 AND f.deleted_at IS NULL
        )
        SELECT uf.id, uf.filename, uf.original_size_bytes, uf.declared_mime, 
               uf.folder_id, COALESCE(ft.folder_path, '') as folder_path
        FROM user_files uf
        JOIN folder_tree ft ON uf.folder_id = ft.id
        WHERE uf.user_id = $2 AND uf.deleted_at IS NULL
        ORDER BY folder_path, uf.filename
        LIMIT $3
    `, folderID, userID, maxFilesPerQuery)
	if err != nil {
		return fmt.Errorf("recursive file query failed: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var item ShareItem
		var size int64
		var folderPath, parentFolderID string
		if err := rows.Scan(&item.ID, &item.Name, &size, &item.MimeType, &parentFolderID, &folderPath); err != nil {
			logger.L.Error("failed to scan recursive file", zap.Error(err))
			continue
		}
		item.Type = "file"
		item.Size = &size
		item.ParentID = &parentFolderID
		if folderPath == "" {
			item.Path = item.Name
		} else {
			item.Path = folderPath + "/" + item.Name
		}
		*items = append(*items, item)
	}
	return rows.Err()
}

func (h *Handler) ResolveFolderShare() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		token := ctx.Param("token")
		cacheKey := cache.ShareResolveKey(token)
		if h.Cache != nil {
			var cached FolderShareListing
			if err := h.Cache.Get(ctx.Request.Context(), cacheKey, &cached); err == nil {
				accessEvent := worker.FolderShareEvent{Type: "accessed", ShareID: cached.Share.ID, FolderID: cached.Share.FolderID, Token: token, IPAddress: ctx.ClientIP(), Timestamp: time.Now().Format(time.RFC3339)}
				_ = h.Producer.PublishFolderShareEvent(ctx.Request.Context(), accessEvent)
				ctx.JSON(http.StatusOK, cached)
				return
			}
		}

		var shareID, targetID, title, description string
		var recursive, snapshotMode bool
		var expiresAt sql.NullTime
		var createdAt time.Time
		err := h.DB.QueryRowContext(ctx.Request.Context(), `
            SELECT id, target_id, title, description, recursive, snapshot_mode, expires_at, created_at
            FROM shares
            WHERE token=$1 AND target_type='folder' AND revoked=false
        `, token).Scan(&shareID, &targetID, &title, &description, &recursive, &snapshotMode, &expiresAt, &createdAt)
		if err != nil {
			if err == sql.ErrNoRows {
				ctx.JSON(http.StatusNotFound, gin.H{"error": "share not found"})
				return
			}
			logger.L.Error("db error", zap.Error(err))
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			return
		}
		if expiresAt.Valid && expiresAt.Time.Before(time.Now()) {
			ctx.JSON(http.StatusGone, gin.H{"error": "share expired"})
			return
		}

		var folderName string
		if err := h.DB.QueryRowContext(ctx.Request.Context(), "SELECT name FROM folders WHERE id=$1", targetID).Scan(&folderName); err != nil {
			logger.L.Error("folder not found", zap.Error(err))
			ctx.JSON(http.StatusNotFound, gin.H{"error": "folder not found"})
			return
		}
		shareInfo := FolderShareResponse{ID: shareID, Token: token, URL: "/fs/" + token, FolderID: targetID, FolderName: folderName, Title: title, Description: description, Recursive: recursive, SnapshotMode: snapshotMode, CreatedAt: createdAt}
		if expiresAt.Valid {
			shareInfo.ExpiresAt = &expiresAt.Time
		}

		var items []ShareItem
		var total int
		var ownerID string
		if err := h.DB.QueryRowContext(ctx.Request.Context(), `SELECT user_id FROM folders WHERE id = $1 AND deleted_at IS NULL`, targetID).Scan(&ownerID); err != nil {
			logger.L.Error("failed to get folder owner", zap.Error(err))
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			return
		}
		if recursive {
			items, total = h.getRecursiveItemsStandalone(ctx.Request.Context(), targetID, ownerID)
		} else {
			items, total = h.getDirectItemsStandalone(ctx.Request.Context(), targetID, ownerID)
		}
		listing := FolderShareListing{Share: shareInfo, Items: items, Total: total, HasMore: false}

		accessEvent := worker.FolderShareEvent{Type: "accessed", ShareID: shareID, FolderID: targetID, Token: token, IPAddress: ctx.ClientIP(), Timestamp: time.Now().Format(time.RFC3339)}
		_ = h.Producer.PublishFolderShareEvent(ctx.Request.Context(), accessEvent)
		if h.Cache != nil {
			ttl := 2 * time.Minute
			if expiresAt.Valid {
				if remaining := time.Until(expiresAt.Time); remaining < ttl {
					ttl = remaining
				}
			}
			_ = h.Cache.Set(ctx.Request.Context(), cacheKey, listing, ttl)
		}
		ctx.JSON(http.StatusOK, listing)
	}
}

func (h *Handler) ResolveFolderShareContents() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		token := ctx.Param("token")
		var shareID, targetID string
		var recursive bool
		var expiresAt sql.NullTime
		err := h.DB.QueryRowContext(ctx.Request.Context(), `
            SELECT id, target_id, recursive, expires_at
            FROM shares
            WHERE token=$1 AND target_type='folder' AND revoked=false
        `, token).Scan(&shareID, &targetID, &recursive, &expiresAt)
		if err != nil {
			if err == sql.ErrNoRows {
				ctx.JSON(http.StatusNotFound, gin.H{"error": "share not found"})
				return
			}
			logger.L.Error("share lookup failed", zap.Error(err))
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			return
		}
		if expiresAt.Valid && expiresAt.Time.Before(time.Now()) {
			ctx.JSON(http.StatusGone, gin.H{"error": "share expired"})
			return
		}

		folderId := ctx.Query("folderId")
		if folderId == "" {
			folderId = targetID
		}
		var ownerID string
		if err := h.DB.QueryRowContext(ctx.Request.Context(), `SELECT user_id FROM folders WHERE id = $1 AND deleted_at IS NULL`, targetID).Scan(&ownerID); err != nil {
			logger.L.Error("failed to get folder owner", zap.Error(err))
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			return
		}

		if !recursive {
			if folderId != targetID {
				var parentID sql.NullString
				if err := h.DB.QueryRowContext(ctx.Request.Context(), `SELECT parent_id FROM folders WHERE id=$1 AND deleted_at IS NULL`, folderId).Scan(&parentID); err != nil {
					logger.L.Warn("parent lookup failed", zap.Error(err))
					ctx.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
					return
				}
				if !parentID.Valid || parentID.String != targetID {
					ctx.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
					return
				}
			}
		} else {
			var count int
			if err := h.DB.QueryRowContext(ctx.Request.Context(), `
                WITH RECURSIVE folder_tree AS (
                    SELECT id FROM folders WHERE id = $1 AND deleted_at IS NULL
                    UNION ALL
                    SELECT f.id FROM folders f
                    INNER JOIN folder_tree ft ON f.parent_id = ft.id
                    WHERE f.deleted_at IS NULL
                )
                SELECT COUNT(*) FROM folder_tree WHERE id = $2
            `, targetID, folderId).Scan(&count); err != nil || count == 0 {
				ctx.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
				return
			}
		}

		items, total := h.getDirectItemsStandalone(ctx.Request.Context(), folderId, ownerID)
		var folderName string
		_ = h.DB.QueryRowContext(ctx.Request.Context(), `SELECT name FROM folders WHERE id=$1`, targetID).Scan(&folderName)
		listing := FolderShareListing{Share: FolderShareResponse{ID: shareID, Token: token, URL: "/fs/" + token, FolderID: targetID, FolderName: folderName, Recursive: recursive}, Items: items, Total: total, HasMore: false}
		ctx.JSON(http.StatusOK, listing)
	}
}

func (h *Handler) ResolveFolderShareAncestors() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		token := ctx.Param("token")
		var shareID, targetID string
		var recursive bool
		var expiresAt sql.NullTime
		err := h.DB.QueryRowContext(ctx.Request.Context(), `
            SELECT id, target_id, recursive, expires_at
            FROM shares
            WHERE token=$1 AND target_type='folder' AND revoked=false
        `, token).Scan(&shareID, &targetID, &recursive, &expiresAt)
		if err != nil {
			if err == sql.ErrNoRows {
				ctx.JSON(http.StatusNotFound, gin.H{"error": "share not found"})
				return
			}
			logger.L.Error("share lookup failed", zap.Error(err))
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			return
		}
		if expiresAt.Valid && expiresAt.Time.Before(time.Now()) {
			ctx.JSON(http.StatusGone, gin.H{"error": "share expired"})
			return
		}

		folderId := ctx.Query("folderId")
		if folderId == "" {
			folderId = targetID
		}
		if !recursive {
			if folderId != targetID {
				var p sql.NullString
				if err := h.DB.QueryRowContext(ctx.Request.Context(), `SELECT parent_id FROM folders WHERE id=$1 AND deleted_at IS NULL`, folderId).Scan(&p); err != nil || !p.Valid || p.String != targetID {
					ctx.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
					return
				}
			}
		} else {
			var count int
			if err := h.DB.QueryRowContext(ctx.Request.Context(), `
                WITH RECURSIVE folder_tree AS (
                    SELECT id FROM folders WHERE id = $1 AND deleted_at IS NULL
                    UNION ALL
                    SELECT f.id FROM folders f
                    INNER JOIN folder_tree ft ON f.parent_id = ft.id
                    WHERE f.deleted_at IS NULL
                )
                SELECT COUNT(*) FROM folder_tree WHERE id = $2
            `, targetID, folderId).Scan(&count); err != nil || count == 0 {
				ctx.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
				return
			}
		}

		rows, err := h.DB.QueryContext(ctx.Request.Context(), `
            WITH RECURSIVE anc AS (
                SELECT id, name, parent_id
                FROM folders
                WHERE id = $1 AND deleted_at IS NULL
                UNION ALL
                SELECT f.id, f.name, f.parent_id
                FROM folders f
                INNER JOIN anc a ON f.id = a.parent_id
                WHERE f.deleted_at IS NULL
            )
            SELECT id, name FROM anc WHERE id != $1
        `, folderId)
		if err != nil {
			logger.L.Error("ancestor query failed", zap.Error(err))
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			return
		}
		defer rows.Close()
		var ancestors []map[string]string
		for rows.Next() {
			var id, name string
			if err := rows.Scan(&id, &name); err == nil {
				ancestors = append([]map[string]string{{"id": id, "name": name}}, ancestors...)
			}
		}
		ctx.JSON(http.StatusOK, gin.H{"ancestors": ancestors})
	}
}

func (h *Handler) DownloadFromShare() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		token := ctx.Param("token")
		fileID := ctx.Param("fileId")
		var shareID, targetFolderID string
		var recursive bool
		var expiresAt sql.NullTime
		err := h.DB.QueryRowContext(ctx.Request.Context(), `
            SELECT id, target_id, recursive, expires_at
            FROM shares
            WHERE token=$1 AND target_type='folder' AND revoked=false
        `, token).Scan(&shareID, &targetFolderID, &recursive, &expiresAt)
		if err != nil {
			if err == sql.ErrNoRows {
				ctx.JSON(http.StatusNotFound, gin.H{"error": "share not found"})
				return
			}
			logger.L.Error("share lookup failed", zap.Error(err))
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			return
		}
		if expiresAt.Valid && expiresAt.Time.Before(time.Now()) {
			ctx.JSON(http.StatusGone, gin.H{"error": "share expired"})
			return
		}

		var blobKey, filename string
		var fileFolderID sql.NullString
		var size int64
		err = h.DB.QueryRowContext(ctx.Request.Context(), `
            SELECT uf.filename, fc.blob_key, fc.size_bytes, uf.folder_id
            FROM user_files uf
            JOIN file_contents fc ON uf.content_id = fc.id
            WHERE uf.id = $1 AND uf.deleted_at IS NULL
        `, fileID).Scan(&filename, &blobKey, &size, &fileFolderID)
		if err != nil {
			if err == sql.ErrNoRows {
				ctx.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
				return
			}
			logger.L.Error("file query failed", zap.Error(err))
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			return
		}
		if !fileFolderID.Valid {
			ctx.JSON(http.StatusForbidden, gin.H{"error": "file not accessible"})
			return
		}

		accessible := false
		if fileFolderID.String == targetFolderID {
			accessible = true
		} else if recursive {
			var count int
			err = h.DB.QueryRowContext(ctx.Request.Context(), `
                WITH RECURSIVE folder_tree AS (
                    SELECT id FROM folders WHERE id = $1 AND deleted_at IS NULL
                    UNION ALL
                    SELECT f.id FROM folders f
                    INNER JOIN folder_tree ft ON f.parent_id = ft.id
                    WHERE f.deleted_at IS NULL
                )
                SELECT COUNT(*) FROM folder_tree WHERE id = $2
            `, targetFolderID, fileFolderID.String).Scan(&count)
			if err == nil && count > 0 {
				accessible = true
			}
		}
		if !accessible {
			ctx.JSON(http.StatusForbidden, gin.H{"error": "file not accessible through this share"})
			return
		}

		downloadURL, err := h.Storage.PresignedGetURL(ctx.Request.Context(), blobKey, int64(presignedURLTTL/time.Minute))
		if err != nil {
			logger.L.Error("presigned URL generation failed", zap.Error(err))
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "download URL generation failed"})
			return
		}

		downloadEvent := worker.FolderShareEvent{Type: "file_downloaded", ShareID: shareID, FolderID: targetFolderID, Token: token, IPAddress: ctx.ClientIP(), Timestamp: time.Now().Format(time.RFC3339)}
		_ = h.Producer.PublishFolderShareEvent(ctx.Request.Context(), downloadEvent)
		ctx.JSON(http.StatusOK, gin.H{"downloadUrl": downloadURL, "filename": filename, "size": size, "expiresAt": time.Now().Add(presignedURLTTL)})
	}
}

func (h *Handler) DownloadFolderArchive() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		token := ctx.Param("token")
		var shareID, targetFolderID, folderName string
		var recursive bool
		var expiresAt sql.NullTime
		err := h.DB.QueryRowContext(ctx.Request.Context(), `
            SELECT s.id, s.target_id, s.recursive, s.expires_at, f.name
            FROM shares s
            JOIN folders f ON s.target_id = f.id
            WHERE s.token=$1 AND s.target_type='folder' AND s.revoked=false
        `, token).Scan(&shareID, &targetFolderID, &recursive, &expiresAt, &folderName)
		if err != nil {
			if err == sql.ErrNoRows {
				ctx.JSON(http.StatusNotFound, gin.H{"error": "share not found"})
				return
			}
			logger.L.Error("share lookup failed", zap.Error(err))
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			return
		}
		if expiresAt.Valid && expiresAt.Time.Before(time.Now()) {
			ctx.JSON(http.StatusGone, gin.H{"error": "share expired"})
			return
		}

		var items []ShareItem
		var ownerID string
		if err := h.DB.QueryRowContext(ctx.Request.Context(), `SELECT user_id FROM folders WHERE id = $1 AND deleted_at IS NULL`, targetFolderID).Scan(&ownerID); err != nil {
			logger.L.Error("failed to get folder owner for download", zap.Error(err))
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			return
		}
		if recursive {
			items, _ = h.getRecursiveItemsStandalone(ctx.Request.Context(), targetFolderID, ownerID)
		} else {
			items, _ = h.getDirectItemsStandalone(ctx.Request.Context(), targetFolderID, ownerID)
		}

		var files []ShareItem
		for _, it := range items {
			if it.Type == "file" {
				files = append(files, it)
			}
		}
		if len(files) == 0 {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "no files found in shared folder"})
			return
		}

		zipFilename := fmt.Sprintf("%s.zip", folderName)
		ctx.Header("Content-Type", "application/zip")
		ctx.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", zipFilename))
		ctx.Header("Cache-Control", "no-cache")

		zipWriter := zip.NewWriter(ctx.Writer)
		defer zipWriter.Close()
		for _, file := range files {
			if err := h.addFileToZip(ctx.Request.Context(), zipWriter, file); err != nil {
				logger.L.Error("failed to add file to zip", zap.String("fileId", file.ID), zap.String("fileName", file.Name), zap.Error(err))
				continue
			}
		}

		downloadEvent := worker.FolderShareEvent{Type: "folder_downloaded", ShareID: shareID, FolderID: targetFolderID, Token: token, IPAddress: ctx.ClientIP(), Timestamp: time.Now().Format(time.RFC3339)}
		_ = h.Producer.PublishFolderShareEvent(ctx.Request.Context(), downloadEvent)
	}
}

func (h *Handler) addFileToZip(ctx context.Context, zipWriter *zip.Writer, file ShareItem) error {
	var blobKey string
	if err := h.DB.QueryRowContext(ctx, `
        SELECT fc.blob_key
        FROM user_files uf
        JOIN file_contents fc ON uf.content_id = fc.id
        WHERE uf.id = $1 AND uf.deleted_at IS NULL
    `, file.ID).Scan(&blobKey); err != nil {
		return fmt.Errorf("failed to get blob key: %w", err)
	}

	downloadURL, err := h.Storage.PresignedGetURL(ctx, blobKey, int64((5*time.Minute)/time.Minute))
	if err != nil {
		return fmt.Errorf("failed to get presigned URL: %w", err)
	}

	resp, err := http.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("failed to download file: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("file download failed with status: %d", resp.StatusCode)
	}

	zipFile, err := zipWriter.Create(file.Path)
	if err != nil {
		return fmt.Errorf("failed to create zip entry: %w", err)
	}
	if _, err = io.Copy(zipFile, resp.Body); err != nil {
		return fmt.Errorf("failed to copy file content: %w", err)
	}
	return nil
}

func (h *Handler) getDirectItemsStandalone(ctx context.Context, folderID, userID string) ([]ShareItem, int) {
	var items []ShareItem
	if err := h.addSubfolders(ctx, &items, folderID, userID); err != nil {
		logger.L.Error("getDirectItemsStandalone failed (folders)", zap.Error(err))
		return []ShareItem{}, 0
	}
	if err := h.addFiles(ctx, &items, folderID, userID); err != nil {
		logger.L.Error("getDirectItemsStandalone failed (files)", zap.Error(err))
		return []ShareItem{}, 0
	}
	return items, len(items)
}

func (h *Handler) getRecursiveItemsStandalone(ctx context.Context, folderID, userID string) ([]ShareItem, int) {
	var items []ShareItem
	if err := h.addRecursiveFolders(ctx, &items, folderID, userID); err != nil {
		logger.L.Error("getRecursiveItemsStandalone failed (folders)", zap.Error(err))
		return []ShareItem{}, 0
	}
	if err := h.addRecursiveFiles(ctx, &items, folderID, userID); err != nil {
		logger.L.Error("getRecursiveItemsStandalone failed (files)", zap.Error(err))
		return []ShareItem{}, 0
	}
	return items, len(items)
}

func (h *Handler) CreateFolderShare() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := auth.GetUserIDFromCtx(c.Request.Context())
		var req CreateFolderShareReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request payload"})
			return
		}
		folderName, err := h.validateFolderOwnership(c.Request.Context(), req.FolderID, userID)
		if err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusNotFound, gin.H{"error": "folder not found"})
				return
			}
			logger.L.Error("folder validation failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			return
		}

		token, err := generateShareToken()
		if err != nil {
			logger.L.Error("token generation failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			return
		}
		shareID, createdAt, err := h.createShare(c.Request.Context(), token, userID, req)
		if err != nil {
			logger.L.Error("share creation failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			return
		}
		if req.SnapshotMode && req.Recursive {
			h.enqueueSnapshotJob(c.Request.Context(), shareID, req.FolderID, token, createdAt)
		}
		h.publishShareEvent(c.Request.Context(), "created", shareID, req.FolderID, token, userID, "", createdAt)
		h.invalidateShareCache(c.Request.Context(), token)
		response := FolderShareResponse{ID: shareID, Token: token, URL: "/fs/" + token, FolderID: req.FolderID, FolderName: folderName, Title: req.Title, Description: req.Description, Recursive: req.Recursive, SnapshotMode: req.SnapshotMode, ExpiresAt: req.ExpiresAt, CreatedAt: createdAt}
		c.JSON(http.StatusCreated, response)
	}
}

func (h *Handler) GetShareInfo() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := auth.GetUserIDFromCtx(c.Request.Context())
		shareID := c.Param("id")
		if shareID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "share ID is required"})
			return
		}
		share, err := h.getShareByID(c.Request.Context(), shareID)
		if err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusNotFound, gin.H{"error": "share not found"})
				return
			}
			logger.L.Error("share lookup failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			return
		}
		if share.CreatorID != userID {
			c.JSON(http.StatusForbidden, gin.H{"error": "not authorized"})
			return
		}
		response := h.buildShareResponse(share)
		c.JSON(http.StatusOK, response)
	}
}
