package rest

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"

	"backend/internal/audit"
	"backend/internal/auth"
	"backend/internal/storage"
	"backend/internal/utils"
	"backend/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"go.uber.org/zap"
)

func RegisterUploadRoutes(rg *gin.RouterGroup, db *sql.DB, st *storage.MinioStorage, jwtSecret string) {
	uploads := rg.Group("/uploads")
	uploads.Use(auth.RequireAuth(jwtSecret))
	h := &uploadHandler{
		db:      db,
		storage: st,
	}
	uploads.POST("/session", h.createSession)
	uploads.POST("/complete", h.complete)
	uploads.POST("/abort", h.abort)
}

type uploadHandler struct {
	db      *sql.DB
	storage *storage.MinioStorage
}

type createSessionRequest struct {
	Filename     string `json:"filename" binding:"required"`
	DeclaredMime string `json:"declaredMime"`
	OriginalSize int64  `json:"originalSize" binding:"required"`
	ClientSha256 string `json:"clientSha256"`
}

type createSessionResponse struct {
	SessionId      string `json:"sessionId,omitempty"`
	UploadUrl      string `json:"uploadUrl,omitempty"`
	TempBlobKey    string `json:"tempBlobKey,omitempty"`
	SkipUpload     bool   `json:"skipUpload"`
	ExistingFileId string `json:"existingFileId,omitempty"`
	UserFileId     string `json:"userFileId,omitempty"`
}

func (h *uploadHandler) createSession(c *gin.Context) {
	var req createSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}

	if req.OriginalSize > 0 {
		used, quota, err := h.getUsageAndQuota(c.Request.Context(), userID)
		if err != nil {
			logger.L.Error("quota lookup failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
			return
		}
		if used+req.OriginalSize > quota {
			c.JSON(http.StatusForbidden, gin.H{
				"error":      "quota exceeded",
				"usedBytes":  used,
				"quotaBytes": quota,
				"attempt":    req.OriginalSize,
			})
			return
		}
	}

	if req.ClientSha256 != "" {
		var contentID string
		var sizeBytes int64
		err := h.db.QueryRowContext(c.Request.Context(),
			"SELECT id, size_bytes FROM file_contents WHERE content_hash=$1 LIMIT 1",
			req.ClientSha256).Scan(&contentID, &sizeBytes)

		if err == nil {
			var existingUserFile string
			err = h.db.QueryRowContext(c.Request.Context(),
				"SELECT id FROM user_files WHERE user_id=$1 AND content_id=$2 AND deleted_at IS NULL LIMIT 1",
				userID, contentID).Scan(&existingUserFile)
			if err == nil {
				c.JSON(http.StatusOK, createSessionResponse{
					SkipUpload:     true,
					ExistingFileId: existingUserFile,
					UserFileId:     existingUserFile,
				})
				return
			}

			tx, txErr := h.db.BeginTx(c.Request.Context(), nil)
			if txErr != nil {
				logger.L.Error("fastpath tx begin failed", zap.Error(txErr))
				c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
				return
			}
			defer tx.Rollback()

			var newUserFileID string
			err = tx.QueryRowContext(c.Request.Context(), `
				INSERT INTO user_files (id, user_id, content_id, filename, declared_mime, original_size_bytes, created_at, updated_at)
				VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, now(), now())
				RETURNING id
			`, userID, contentID, req.Filename, req.DeclaredMime, req.OriginalSize).Scan(&newUserFileID)
			if err != nil {
				logger.L.Error("fastpath insert user_files failed", zap.Error(err))
				c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
				return
			}
			if _, err := tx.ExecContext(c.Request.Context(),
				"UPDATE file_contents SET ref_count = ref_count + 1 WHERE id=$1", contentID); err != nil {
				logger.L.Error("fastpath refcount inc failed", zap.Error(err))
				c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
				return
			}
			if err := tx.Commit(); err != nil {
				logger.L.Error("fastpath tx commit failed", zap.Error(err))
				c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
				return
			}

			c.JSON(http.StatusOK, createSessionResponse{
				SkipUpload:     true,
				ExistingFileId: newUserFileID,
				UserFileId:     newUserFileID,
			})
			return
		}
	}

	sessionID := uuid.NewString()
	tempName := fmt.Sprintf("tmp/%s/%s", sessionID, req.Filename)
	url, err := h.storage.PresignedPutURL(c.Request.Context(), tempName, 30)
	if err != nil {
		logger.L.Error("presigned url fail", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed generate upload url"})
		return
	}
	_, err = h.db.ExecContext(c.Request.Context(), `
		INSERT INTO upload_sessions (id, user_id, filename, declared_mime, original_size_bytes, temp_blob_key, client_sha256, status, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,'OPEN',now(),now())
	`, sessionID, userID, req.Filename, req.DeclaredMime, req.OriginalSize, tempName, req.ClientSha256)
	if err != nil {
		logger.L.Error("insert upload session failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed create session"})
		return
	}
	c.JSON(http.StatusCreated, createSessionResponse{
		SessionId:   sessionID,
		UploadUrl:   url,
		TempBlobKey: tempName,
		SkipUpload:  false,
	})
}

type completeRequest struct {
	SessionId    string `json:"sessionId" binding:"required"`
	ClientSha256 string `json:"clientSha256"`
}

func (h *uploadHandler) complete(c *gin.Context) {
	var req completeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}

	var tempKey, filename, declaredMime, clientSha string
	var originalSize int64
	var status string
	err := h.db.QueryRowContext(c.Request.Context(),
		"SELECT temp_blob_key, filename, declared_mime, original_size_bytes, status, client_sha256 FROM upload_sessions WHERE id=$1",
		req.SessionId).Scan(&tempKey, &filename, &declaredMime, &originalSize, &status, &clientSha)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
			return
		}
		logger.L.Error("db err", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}
	if status != "OPEN" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session not open"})
		return
	}

	sha := req.ClientSha256
	if sha == "" && clientSha != "" {
		sha = clientSha
	}

	objReader, err := h.storage.GetObjectReader(c.Request.Context(), tempKey)
	if err != nil {
		logger.L.Error("get temp object failed", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "uploaded object not found"})
		return
	}
	head := make([]byte, 512)
	n, _ := io.ReadFull(objReader, head)

	if sha == "" {
		contentReader := io.MultiReader(bytes.NewReader(head[:n]), objReader)
		computed, err := utils.ComputeSHA256(contentReader)
		if err != nil {
			_ = objReader.Close()
			logger.L.Error("hash compute failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "hash compute failed"})
			return
		}
		sha = computed
	}
	_ = objReader.Close()

	sniffedMime := http.DetectContentType(head[:n])
	if declaredMime != "" && declaredMime != sniffedMime {
		ext := filepath.Ext(filename)
		alias := mime.TypeByExtension(ext)
		if !(alias == sniffedMime || alias == declaredMime) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":        "declared mime mismatch",
				"declaredMime": declaredMime,
				"sniffedMime":  sniffedMime,
			})
			return
		}
	}
	if declaredMime == "" {
		declaredMime = sniffedMime
	}

	tx, err := h.db.BeginTx(c.Request.Context(), nil)
	if err != nil {
		logger.L.Error("tx begin failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	defer tx.Rollback()

	var contentID string
	err = tx.QueryRowContext(c.Request.Context(),
		"SELECT id FROM file_contents WHERE content_hash=$1 LIMIT 1",
		sha).Scan(&contentID)

	if err == nil {
		var newUserFileID string
		err = tx.QueryRowContext(c.Request.Context(), `
			INSERT INTO user_files (id, user_id, content_id, filename, declared_mime, original_size_bytes, created_at, updated_at)
			VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, now(), now())
			RETURNING id
		`, userID, contentID, filename, declaredMime, originalSize).Scan(&newUserFileID)
		if err != nil {
			logger.L.Error("insert user_files dedup failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
			return
		}
		if _, err := tx.ExecContext(c.Request.Context(),
			"UPDATE file_contents SET ref_count = ref_count + 1 WHERE id=$1", contentID); err != nil {
			logger.L.Error("refcount inc failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
			return
		}
		_, _ = tx.ExecContext(c.Request.Context(), "UPDATE upload_sessions SET status='COMPLETED', client_sha256=$2, updated_at=now() WHERE id=$1", req.SessionId, sha)
		if err := tx.Commit(); err != nil {
			logger.L.Error("commit failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"userFileId": newUserFileID, "deduped": true})
		return
	}

	used, quota, errQ := h.getUsageAndQuota(c.Request.Context(), userID)
	if errQ != nil {
		logger.L.Error("quota lookup failed", zap.Error(errQ))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	if used+originalSize > quota {
		c.JSON(http.StatusForbidden, gin.H{
			"error":      "quota exceeded",
			"usedBytes":  used,
			"quotaBytes": quota,
			"attempt":    originalSize,
		})
		return
	}

	finalKey := fmt.Sprintf("objects/%s", sha)
	if err := h.storage.CopyTempToObject(c.Request.Context(), tempKey, finalKey); err != nil {
		logger.L.Error("move temp to final failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "storage move failed"})
		return
	}

	var newContentID string
	err = tx.QueryRowContext(c.Request.Context(), `
		INSERT INTO file_contents (id, content_hash, blob_key, size_bytes, mime_type, ref_count, created_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, 1, now())
		RETURNING id
	`, sha, finalKey, originalSize, declaredMime).Scan(&newContentID)
	if err != nil {
		logger.L.Warn("insert file_contents failed, fallback to select", zap.Error(err))
		err2 := tx.QueryRowContext(c.Request.Context(),
			"SELECT id FROM file_contents WHERE content_hash=$1 LIMIT 1", sha).Scan(&newContentID)
		if err2 != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
			return
		}
		if _, err := tx.ExecContext(c.Request.Context(),
			"UPDATE file_contents SET ref_count = ref_count + 1 WHERE id=$1", newContentID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
			return
		}
	}

	var newUserFileID string
	err = tx.QueryRowContext(c.Request.Context(), `
		INSERT INTO user_files (id, user_id, content_id, filename, declared_mime, original_size_bytes, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, now(), now())
		RETURNING id
	`, userID, newContentID, filename, declaredMime, originalSize).Scan(&newUserFileID)
	if err != nil {
		logger.L.Error("insert user_files final failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}

	_, _ = tx.ExecContext(c.Request.Context(), "UPDATE upload_sessions SET status='COMPLETED', client_sha256=$2, updated_at=now() WHERE id=$1", req.SessionId, sha)
	_ = audit.Log(
		c.Request.Context(),
		h.db,
		userID,
		"upload",
		"file",
		newUserFileID,
		map[string]interface{}{
			"filename": filename,
			"size":     originalSize,
			"sha256":   sha,
		},
	)

	if err := tx.Commit(); err != nil {
		logger.L.Error("tx commit failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"userFileId": newUserFileID, "contentId": newContentID, "deduped": false})
}

type abortRequest struct {
	SessionId string `json:"sessionId" binding:"required"`
}

func (h *uploadHandler) abort(c *gin.Context) {
	var req abortRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	var tempKey string
	err := h.db.QueryRowContext(c.Request.Context(),
		"SELECT temp_blob_key FROM upload_sessions WHERE id=$1 AND user_id=$2", req.SessionId, userID).Scan(&tempKey)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}
	err = h.storage.Client.RemoveObject(c.Request.Context(), h.storage.Bucket, tempKey, minio.RemoveObjectOptions{})
	if err != nil {
		logger.L.Warn("remove temp object failed", zap.Error(err))
	}
	_, _ = h.db.ExecContext(c.Request.Context(), "UPDATE upload_sessions SET status='ABORTED', updated_at=now() WHERE id=$1", req.SessionId)
	c.JSON(http.StatusOK, gin.H{"message": "aborted"})
}

func (h *uploadHandler) getUsageAndQuota(ctx context.Context, userID string) (int64, int64, error) {
	var used int64
	if err := h.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(original_size_bytes),0)
		FROM user_files
		WHERE user_id=$1 AND deleted_at IS NULL
	`, userID).Scan(&used); err != nil {
		return 0, 0, err
	}
	var quota int64
	if err := h.db.QueryRowContext(ctx,
		"SELECT quota_bytes FROM users WHERE id=$1", userID).Scan(&quota); err != nil {
		return 0, 0, err
	}
	return used, quota, nil
}
