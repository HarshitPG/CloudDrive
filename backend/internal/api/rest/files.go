package rest

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"backend/internal/audit"
	"backend/internal/auth"
	"backend/internal/cache"
	"backend/internal/storage"
	"backend/internal/worker"
	"backend/pkg/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	//"github.com/google/uuid"
)

func RegisterFileRoutes(rg *gin.RouterGroup, db *sql.DB, st *storage.MinioStorage, jwtSecret string, cache cache.Cache, publish func(ctx context.Context, fileID string, downloadCount int64) error) {
	files := rg.Group("/files")
	files.Use(auth.RequireAuth(jwtSecret))
	producer := worker.NewProducer()
	h := &fileHandler{db: db, storage: st, cache: cache, publish: publish, producer: producer}
	files.GET("", h.listFiles)
	files.GET("/:id", h.getFileMetadata)
	files.GET("/:id/download", h.download)
	files.DELETE("/:id", h.delete)
	files.POST("/:id/restore", h.restore)
	files.PATCH("/:id", h.patch)
	files.POST("/:id/move", h.move)
	files.POST("/:id/versions", h.createVersion)
	files.GET("/:id/versions", h.listVersions)
}

type fileHandler struct {
	db       *sql.DB
	storage  *storage.MinioStorage
	cache    cache.Cache
	publish  func(ctx context.Context, fileID string, downloadCount int64) error
	producer *worker.Producer
}

func (h *fileHandler) download(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	fileId := c.Param("id")

	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}

	var contentBlob, contentID, owner string
	err := h.db.QueryRowContext(c.Request.Context(),
		`SELECT uf.content_id, fc.blob_key, uf.user_id
         FROM user_files uf
         JOIN file_contents fc ON uf.content_id=fc.id
         WHERE uf.id=$1 AND uf.deleted_at IS NULL`,
		fileId,
	).Scan(&contentID, &contentBlob, &owner)

	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}

	allowed := false
	if owner == userID {
		allowed = true
	} else {
		var count int
		err = h.db.QueryRowContext(c.Request.Context(),
			`SELECT COUNT(*) 
             FROM shares 
             WHERE target_type='file' 
               AND target_id=$1 
               AND revoked=false 
               AND (is_public=true OR shared_with_user_id=$2)`,
			fileId, userID,
		).Scan(&count)
		if err == nil && count > 0 {
			allowed = true
		}
	}

	if !allowed {
		c.JSON(http.StatusForbidden, gin.H{"error": "not authorized"})
		return
	}

	url, err := h.storage.PresignedGetURL(c.Request.Context(), contentBlob, 5)
	if err != nil {
		logger.L.Error("presigned get failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed generate download url"})
		return
	}

	var newCount int64
	err = h.db.QueryRowContext(c.Request.Context(),
		"UPDATE user_files SET download_count = download_count + 1 WHERE id=$1 RETURNING download_count",
		fileId,
	).Scan(&newCount)
	if err != nil {
		logger.L.Warn("failed inc download_count", zap.Error(err))
	} else {
		go func(fileID string, count int64) {
			if h.publish != nil {
				_ = h.publish(c.Request.Context(), fileID, count)
			}
		}(fileId, newCount)
	}
	_ = audit.Log(
		c.Request.Context(),
		h.db,
		userID,
		"download",
		"file",
		fileId,
		map[string]interface{}{
			"blobKey": contentBlob,
		},
	)

	c.JSON(http.StatusOK, gin.H{"downloadUrl": url})
}

func (h *fileHandler) delete(c *gin.Context) {
	ctx := c.Request.Context()
	userID := auth.GetUserIDFromCtx(ctx)
	fileId := c.Param("id")

	var owner, contentID, blobKey string
	err := h.db.QueryRowContext(ctx,
		`SELECT uf.user_id, uf.content_id, fc.blob_key
		 FROM user_files uf
		 JOIN file_contents fc ON uf.content_id = fc.id
		 WHERE uf.id=$1 AND uf.deleted_at IS NULL`,
		fileId,
	).Scan(&owner, &contentID, &blobKey)

	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}

	if owner != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "not owner"})
		return
	}

	tx, _ := h.db.BeginTx(ctx, nil)

	if _, err := tx.ExecContext(ctx,
		"UPDATE user_files SET deleted_at = now() WHERE id=$1", fileId); err != nil {
		_ = tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}

	if _, err := tx.ExecContext(ctx,
		"UPDATE file_contents SET ref_count = GREATEST(ref_count - 1, 0) WHERE id=$1", contentID); err != nil {
		_ = tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}

	// TODO: if ref_count == 0, schedule GC job in worker table / job queue
	//       (so MinIO/S3 blob can be deleted later by background worker)

	var refCount int64
	if err := tx.QueryRowContext(ctx,
		"SELECT ref_count FROM file_contents WHERE id=$1", contentID).Scan(&refCount); err == nil {
		if refCount == 0 {
			if h.producer != nil {
				_ = h.producer.PublishGCJob(ctx, worker.GCJob{
					ContentID: contentID,
					BlobKey:   blobKey,
				})
			}
		}
	}

	_, _ = tx.ExecContext(ctx,
		"UPDATE shares SET revoked=true WHERE target_type='file' AND target_id=$1", fileId)

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "commit failed"})
		return
	}

	_ = audit.Log(
		ctx,
		h.db,
		userID,
		"delete",
		"file",
		fileId,
		map[string]interface{}{
			"contentID": contentID,
		},
	)

	// cache invalidation
	var folderId sql.NullString
	_ = h.db.QueryRowContext(ctx,
		"SELECT folder_id FROM user_files WHERE id=$1", fileId).Scan(&folderId)
	if folderId.Valid {
		cache.InvalidateFolder(ctx, h.cache, folderId.String)
	}
	cache.InvalidateFile(ctx, h.cache, fileId)
	cache.InvalidateSearch(ctx, h.cache, userID)

	c.JSON(http.StatusOK, gin.H{"message": "deleted and shares revoked"})
}

func (h *fileHandler) restore(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	fileId := c.Param("id")
	var owner string
	var contentID string
	err := h.db.QueryRowContext(c.Request.Context(),
		"SELECT user_id, content_id FROM user_files WHERE id=$1 AND deleted_at IS NOT NULL", fileId).
		Scan(&owner, &contentID)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "file not found or not in trash"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	if owner != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "not owner"})
		return
	}

	tx, _ := h.db.BeginTx(c.Request.Context(), nil)
	_, err = tx.ExecContext(c.Request.Context(), "UPDATE user_files SET deleted_at = NULL WHERE id=$1", fileId)
	if err != nil {
		_ = tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	_, err = tx.ExecContext(c.Request.Context(), "UPDATE file_contents SET ref_count = ref_count + 1 WHERE id=$1", contentID)
	if err != nil {
		_ = tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}

	_, _ = tx.ExecContext(c.Request.Context(),
		"UPDATE shares SET revoked=false WHERE target_type='file' AND target_id=$1", fileId)

	_ = tx.Commit()
	_ = audit.Log(
		c.Request.Context(),
		h.db,
		userID,
		"restore",
		"file",
		fileId,
		map[string]interface{}{
			"contentID": contentID,
		},
	)

	// cache invalidation
	var folderId sql.NullString
	_ = h.db.QueryRowContext(c.Request.Context(),
		"SELECT folder_id FROM user_files WHERE id=$1", fileId).Scan(&folderId)
	if folderId.Valid {
		cache.InvalidateFolder(c.Request.Context(), h.cache, folderId.String)
	}
	cache.InvalidateFile(c.Request.Context(), h.cache, fileId)
	cache.InvalidateSearch(c.Request.Context(), h.cache, userID)

	c.JSON(http.StatusOK, gin.H{"message": "restored (shares re-enabled)"})
}

type patchRequest struct {
	Filename string   `json:"filename"`
	Tags     []string `json:"tags"`
}

func (h *fileHandler) patch(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	fileId := c.Param("id")
	var owner string
	err := h.db.QueryRowContext(c.Request.Context(), "SELECT user_id FROM user_files WHERE id=$1 AND deleted_at IS NULL", fileId).Scan(&owner)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	if owner != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "not owner"})
		return
	}
	var req patchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Filename != "" {
		_, _ = h.db.ExecContext(c.Request.Context(), "UPDATE user_files SET filename=$1, updated_at=now() WHERE id=$2", req.Filename, fileId)
	}
	if req.Tags != nil {
		tagsJSON, _ := json.Marshal(req.Tags)
		_, _ = h.db.ExecContext(
			c.Request.Context(),
			"UPDATE user_files SET tags=$1, updated_at=now() WHERE id=$2",
			tagsJSON,
			fileId,
		)
	}
	// cache invalidation
	cache.InvalidateFile(c.Request.Context(), h.cache, fileId)
	cache.InvalidateSearch(c.Request.Context(), h.cache, userID)

	c.JSON(http.StatusOK, gin.H{"message": "updated"})
}

type moveRequest struct {
	TargetFolderId string `json:"targetFolderId"`
}

func (h *fileHandler) move(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	fileId := c.Param("id")

	var owner string
	err := h.db.QueryRowContext(c.Request.Context(),
		"SELECT user_id FROM user_files WHERE id=$1 AND deleted_at IS NULL",
		fileId,
	).Scan(&owner)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		return
	}
	if owner != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "not owner"})
		return
	}

	var req moveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var folderOwner string
	err = h.db.QueryRowContext(c.Request.Context(),
		"SELECT user_id FROM folders WHERE id=$1 AND deleted_at IS NULL",
		req.TargetFolderId,
	).Scan(&folderOwner)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "target folder not found"})
		return
	}
	if folderOwner != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "cannot move to folder you don't own"})
		return
	}

	var originalFolderId sql.NullString
	_ = h.db.QueryRowContext(c.Request.Context(),
		"SELECT folder_id FROM user_files WHERE id=$1", fileId).Scan(&originalFolderId)

	_, _ = h.db.ExecContext(c.Request.Context(),
		"UPDATE user_files SET folder_id=$1, updated_at=now() WHERE id=$2",
		req.TargetFolderId, fileId,
	)

	// invalidate
	if h.cache != nil {
		if originalFolderId.Valid {
			cache.InvalidateFolder(c.Request.Context(), h.cache, originalFolderId.String)
		}
		cache.InvalidateFolder(c.Request.Context(), h.cache, req.TargetFolderId)
	}
	cache.InvalidateFile(c.Request.Context(), h.cache, fileId)
	cache.InvalidateSearch(c.Request.Context(), h.cache, userID)
	c.JSON(http.StatusOK, gin.H{"message": "moved"})
}

func (h *fileHandler) createVersion(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	fileId := c.Param("id")
	var owner string
	err := h.db.QueryRowContext(c.Request.Context(), "SELECT user_id FROM user_files WHERE id=$1", fileId).Scan(&owner)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		return
	}
	if owner != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "not owner"})
		return
	}
	var body struct {
		ContentID string `json:"content_id" binding:"required"`
		Filename  string `json:"filename"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	_, _ = h.db.ExecContext(c.Request.Context(), `
		INSERT INTO user_files (id, user_id, content_id, filename, declared_mime, original_size_bytes, version_of, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, $2, $3, null, 0, $4, now(), now())
	`, userID, body.ContentID, body.Filename, fileId)
	c.JSON(http.StatusCreated, gin.H{"message": "version created"})
}

func (h *fileHandler) listVersions(c *gin.Context) {
	fileId := c.Param("id")
	rows, err := h.db.QueryContext(c.Request.Context(), "SELECT id, content_id, filename, created_at FROM user_files WHERE version_of=$1 ORDER BY created_at DESC", fileId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	defer rows.Close()
	var versions []map[string]interface{}
	for rows.Next() {
		var id, contentID, filename string
		var createdAt string
		_ = rows.Scan(&id, &contentID, &filename, &createdAt)
		versions = append(versions, map[string]interface{}{"id": id, "content_id": contentID, "filename": filename, "created_at": createdAt})
	}
	c.JSON(http.StatusOK, gin.H{"versions": versions})
}

func (h *fileHandler) getFileMetadata(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	fileId := c.Param("id")

	var filename, mime, contentHash string
	var size, contentSize, refCount, downloadCount int64
	var createdAt, updatedAt string

	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	// Try cache first
	cacheKey := cache.FileMetadataKey(fileId)
	var cached map[string]interface{}
	if h.cache != nil {
		if err := h.cache.Get(ctx, cacheKey, &cached); err == nil {
			c.JSON(http.StatusOK, cached)
			return
		}
	}

	var folderID *string
	err := h.db.QueryRowContext(c.Request.Context(), `
    SELECT uf.filename, uf.declared_mime, uf.original_size_bytes,
           uf.created_at, uf.updated_at, uf.download_count,
           uf.folder_id,
           fc.content_hash, fc.size_bytes, fc.ref_count
    FROM user_files uf
    JOIN file_contents fc ON uf.content_id = fc.id
    WHERE uf.id=$1 AND uf.user_id=$2 AND uf.deleted_at IS NULL
`, fileId, userID).Scan(&filename, &mime, &size,
		&createdAt, &updatedAt, &downloadCount,
		&folderID,
		&contentHash, &contentSize, &refCount)

	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}

	resp := gin.H{
		"filename":      filename,
		"mime":          mime,
		"size":          size,
		"createdAt":     createdAt,
		"updatedAt":     updatedAt,
		"downloadCount": downloadCount,
		"contentHash":   contentHash,
		"physicalSize":  contentSize,
		"refCount":      refCount,
		"folderId":      folderID,
		"dedupSavings":  size - contentSize,
	}

	// Write-through cache
	if h.cache != nil {
		_ = h.cache.Set(ctx, cacheKey, resp, 5*time.Minute)
	}

	c.JSON(http.StatusOK, resp)
}

func (h *fileHandler) listFiles(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	folderID := c.Query("folderId")
	deleted := c.Query("deleted") == "true"

	var rows *sql.Rows
	var err error
	if deleted {
		// List files in trash for this user
		rows, err = h.db.QueryContext(c.Request.Context(), `
			SELECT uf.id, uf.filename, uf.declared_mime, uf.original_size_bytes,
				   uf.created_at, uf.updated_at, uf.download_count,
				   fc.content_hash, fc.size_bytes, fc.ref_count,
				   uf.deleted_at
			FROM user_files uf
			JOIN file_contents fc ON uf.content_id = fc.id
			WHERE uf.user_id=$1 AND uf.deleted_at IS NOT NULL
			ORDER BY uf.deleted_at DESC
		`, userID)
	} else if folderID != "" {
		rows, err = h.db.QueryContext(c.Request.Context(), `
            SELECT uf.id, uf.filename, uf.declared_mime, uf.original_size_bytes,
                   uf.created_at, uf.updated_at, uf.download_count,
				   fc.content_hash, fc.size_bytes, fc.ref_count
            FROM user_files uf
            JOIN file_contents fc ON uf.content_id = fc.id
            WHERE uf.user_id=$1 AND uf.folder_id=$2 AND uf.deleted_at IS NULL
            ORDER BY uf.created_at DESC
        `, userID, folderID)
	} else {
		rows, err = h.db.QueryContext(c.Request.Context(), `
            SELECT uf.id, uf.filename, uf.declared_mime, uf.original_size_bytes,
                   uf.created_at, uf.updated_at, uf.download_count,
                   fc.content_hash, fc.size_bytes, fc.ref_count
            FROM user_files uf
            JOIN file_contents fc ON uf.content_id = fc.id
            WHERE uf.user_id=$1 AND uf.folder_id IS NULL AND uf.deleted_at IS NULL
            ORDER BY uf.created_at DESC
        `, userID)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}
	defer rows.Close()

	files := []map[string]interface{}{}
	for rows.Next() {
		var id, filename, mime, contentHash string
		var size, contentSize, refCount, downloadCount int64
		var createdAt, updatedAt string
		var deletedAt sql.NullTime
		if deleted {
			if err := rows.Scan(&id, &filename, &mime, &size,
				&createdAt, &updatedAt, &downloadCount,
				&contentHash, &contentSize, &refCount,
				&deletedAt); err != nil {
				continue
			}
		} else {
			if err := rows.Scan(&id, &filename, &mime, &size,
				&createdAt, &updatedAt, &downloadCount,
				&contentHash, &contentSize, &refCount); err != nil {
				continue
			}
		}
		item := map[string]interface{}{
			"id":            id,
			"filename":      filename,
			"mime":          mime,
			"size":          size,
			"createdAt":     createdAt,
			"updatedAt":     updatedAt,
			"downloadCount": downloadCount,
			"contentHash":   contentHash,
			"physicalSize":  contentSize,
			"refCount":      refCount,
			"dedupSavings":  size - contentSize,
		}
		if deleted && deletedAt.Valid {
			item["deletedAt"] = deletedAt.Time
		}
		files = append(files, item)
	}

	c.JSON(http.StatusOK, gin.H{"files": files})
}
