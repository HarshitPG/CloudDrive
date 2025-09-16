package rest

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"backend/internal/auth"
	"backend/internal/storage"
	"backend/pkg/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	//"github.com/google/uuid"
)

func RegisterFileRoutes(rg *gin.RouterGroup, db *sql.DB, st *storage.MinioStorage, jwtSecret string) {
	files := rg.Group("/files")
	files.Use(auth.RequireAuth(jwtSecret))
	h := &fileHandler{db: db, storage: st}
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
	db      *sql.DB
	storage *storage.MinioStorage
}

func (h *fileHandler) download(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	fileId := c.Param("id")
	var contentBlob string
	var contentID string
	var owner string
	err := h.db.QueryRowContext(c.Request.Context(), "SELECT uf.content_id, fc.blob_key, uf.user_id FROM user_files uf JOIN file_contents fc ON uf.content_id=fc.id WHERE uf.id=$1 AND uf.deleted_at IS NULL", fileId).
		Scan(&contentID, &contentBlob, &owner)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	if owner != userID {
		// TODO: check shares table if shared -> allow
		c.JSON(http.StatusForbidden, gin.H{"error": "not owner"})
		return
	}

	url, err := h.storage.PresignedGetURL(c.Request.Context(), contentBlob, 5)
	if err != nil {
		logger.L.Error("presigned get failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed generate download url"})
		return
	}
	_, _ = h.db.ExecContext(c.Request.Context(), "UPDATE user_files SET download_count = download_count + 1 WHERE id=$1", fileId)
	_, _ = h.db.ExecContext(c.Request.Context(), "INSERT INTO audit_logs (user_id, action, target_type, target_id, created_at) VALUES ($1,'download','user_file',$2,now())", userID, fileId)
	c.JSON(http.StatusOK, gin.H{"downloadUrl": url})
}

func (h *fileHandler) delete(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	fileId := c.Param("id")

	var owner string
	var contentID string
	err := h.db.QueryRowContext(c.Request.Context(), "SELECT user_id, content_id FROM user_files WHERE id=$1 AND deleted_at IS NULL", fileId).Scan(&owner, &contentID)
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

	tx, _ := h.db.BeginTx(c.Request.Context(), nil)
	_, err = tx.ExecContext(c.Request.Context(), "UPDATE user_files SET deleted_at = now() WHERE id=$1", fileId)
	if err != nil {
		_ = tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	_, err = tx.ExecContext(c.Request.Context(), "UPDATE file_contents SET ref_count = GREATEST(ref_count - 1, 0) WHERE id=$1", contentID)
	if err != nil {
		_ = tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	// TODO: schedule GC job in worker table / job queue (not implemented here)
	_ = tx.Commit()
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

func (h *fileHandler) restore(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	fileId := c.Param("id")
	var owner string
	var contentID string
	err := h.db.QueryRowContext(c.Request.Context(), "SELECT user_id, content_id FROM user_files WHERE id=$1 AND deleted_at IS NOT NULL", fileId).Scan(&owner, &contentID)
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
	_ = tx.Commit()
	c.JSON(http.StatusOK, gin.H{"message": "restored"})
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

	_, _ = h.db.ExecContext(c.Request.Context(),
		"UPDATE user_files SET folder_id=$1, updated_at=now() WHERE id=$2",
		req.TargetFolderId, fileId,
	)
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

	c.JSON(http.StatusOK, gin.H{
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
	})
}

func (h *fileHandler) listFiles(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	folderID := c.Query("folderId") // optional query param

	var rows *sql.Rows
	var err error
	if folderID != "" {
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
		if err := rows.Scan(&id, &filename, &mime, &size,
			&createdAt, &updatedAt, &downloadCount,
			&contentHash, &contentSize, &refCount); err != nil {
			continue
		}
		files = append(files, map[string]interface{}{
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
		})
	}

	c.JSON(http.StatusOK, gin.H{"files": files})
}
