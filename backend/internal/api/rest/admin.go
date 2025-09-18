package rest

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"backend/internal/auth"
	"backend/internal/worker"
	"backend/pkg/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func RegisterAdminRoutes(rg *gin.RouterGroup, db *sql.DB, jwtSecret string) {
	admin := rg.Group("/admin")
	admin.Use(auth.RequireAuth(jwtSecret))
	h := &adminHandler{db: db}
	admin.GET("/usage", h.adminUsageHandler)
	admin.GET("/audit", h.adminAuditHandler)
	admin.POST("/force-delete", h.adminForceDeleteHandler)
}

type adminHandler struct {
	db *sql.DB
}

func (h *adminHandler) adminUsageHandler(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	rows, err := h.db.QueryContext(ctx, `
		SELECT u.id, u.email, COALESCE(SUM(uf.original_size_bytes),0) AS original_bytes
		FROM users u
		LEFT JOIN user_files uf ON uf.user_id = u.id AND uf.deleted_at IS NULL
		GROUP BY u.id, u.email
		ORDER BY original_bytes DESC
		LIMIT 1000
	`)
	if err != nil {
		logger.L.Error("admin usage query failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	defer rows.Close()

	type outRow struct {
		UserID        string `json:"userId"`
		Email         string `json:"email"`
		OriginalBytes int64  `json:"originalBytes"`
		DedupedBytes  int64  `json:"dedupedBytes"`
	}

	resp := []outRow{}
	for rows.Next() {
		var r outRow
		if scanErr := rows.Scan(&r.UserID, &r.Email, &r.OriginalBytes); scanErr != nil {
			logger.L.Error("admin usage scan failed", zap.Error(scanErr))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal scan error"})
			return
		}
		if err := h.db.QueryRowContext(ctx, `
			SELECT COALESCE(SUM(fc.size_bytes),0)
			FROM file_contents fc
			JOIN (
				SELECT DISTINCT content_id FROM user_files
				WHERE user_id=$1 AND deleted_at IS NULL
			) u ON u.content_id = fc.id
		`, r.UserID).Scan(&r.DedupedBytes); err != nil {
			logger.L.Error("admin deduped query failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
			return
		}
		resp = append(resp, r)
	}
	c.JSON(http.StatusOK, gin.H{"items": resp})
}

func (h *adminHandler) adminAuditHandler(c *gin.Context) {
	limit := 100
	offset := 0
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 1000 {
			limit = n
		} else if n > 1000 {
			limit = 1000
		}
	}
	if v := c.Query("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()

	rows, err := h.db.QueryContext(ctx, `
		SELECT id, user_id, action, target_type, target_id, meta, created_at
		FROM audit_logs
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		logger.L.Error("audit query failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	defer rows.Close()

	items := []map[string]interface{}{}
	for rows.Next() {
		var id string
		var userID sql.NullString
		var action, targetType sql.NullString
		var targetID sql.NullString
		var meta sql.NullString
		var createdAt time.Time
		if sc := rows.Scan(&id, &userID, &action, &targetType, &targetID, &meta, &createdAt); sc != nil {
			logger.L.Error("audit row scan failed", zap.Error(sc))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
			return
		}
		items = append(items, map[string]interface{}{
			"id":         id,
			"userId":     nullableToString(userID),
			"action":     nullableToString(action),
			"targetType": nullableToString(targetType),
			"targetId":   nullableToString(targetID),
			"meta":       nullableToString(meta),
			"createdAt":  createdAt.Format(time.RFC3339),
		})
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *adminHandler) adminForceDeleteHandler(c *gin.Context) {
	var body struct {
		UserFileID string `json:"userFileId"`
		ContentID  string `json:"contentId"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	tx, err := h.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		logger.L.Error("force-delete tx begin failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	defer tx.Rollback()

	if body.UserFileID != "" {
		var contentID string
		err := tx.QueryRowContext(ctx, "SELECT content_id FROM user_files WHERE id=$1", body.UserFileID).Scan(&contentID)
		if err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusNotFound, gin.H{"error": "user_file not found"})
				return
			}
			logger.L.Error("force-delete lookup failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
			return
		}

		if _, err := tx.ExecContext(ctx, "DELETE FROM user_files WHERE id=$1", body.UserFileID); err != nil {
			logger.L.Error("force-delete delete user_files failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
			return
		}
		if _, err := tx.ExecContext(ctx, "UPDATE file_contents SET ref_count = GREATEST(ref_count - 1,0) WHERE id=$1", contentID); err != nil {
			logger.L.Error("force-delete update ref_count failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
			return
		}

		var refCount int64
		if err := tx.QueryRowContext(ctx, "SELECT ref_count FROM file_contents WHERE id=$1", contentID).Scan(&refCount); err == nil {
			if refCount == 0 {
				var blobKey string
				if err := tx.QueryRowContext(ctx, "SELECT blob_key FROM file_contents WHERE id=$1", contentID).Scan(&blobKey); err == nil {
					_, _ = tx.ExecContext(ctx, "DELETE FROM file_contents WHERE id=$1", contentID)
					// TODO: schedule background job to delete object from storage (MinIO/Azure Blob)
					// insert into worker_jobs table or publish to Kafka/Redis queue for worker to delete blobKey
					producer := worker.NewProducer()
					defer producer.Close()

					_ = producer.PublishGCJob(ctx, worker.GCJob{
						ContentID: contentID,
						BlobKey:   blobKey,
					})

					_, _ = tx.ExecContext(ctx, "INSERT INTO audit_logs (user_id, action, target_type, target_id, meta, created_at) VALUES ($1,'force_delete','content',$2,$3,now())", auth.GetUserIDFromCtx(c.Request.Context()), contentID, mapToJSON(map[string]string{"blobKey": blobKey}))
				}
			}
		}
	} else if body.ContentID != "" {
		if _, err := tx.ExecContext(ctx, "DELETE FROM user_files WHERE content_id=$1", body.ContentID); err != nil {
			logger.L.Error("force-delete delete user_files by content failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
			return
		}
		var blobKey string
		if err := tx.QueryRowContext(ctx, "SELECT blob_key FROM file_contents WHERE id=$1", body.ContentID).Scan(&blobKey); err == nil {
			_, _ = tx.ExecContext(ctx, "DELETE FROM file_contents WHERE id=$1", body.ContentID)
			// TODO: schedule background deletion of blobKey
			producer := worker.NewProducer()
			defer producer.Close()

			_ = producer.PublishGCJob(ctx, worker.GCJob{
				ContentID: body.ContentID,
				BlobKey:   blobKey,
			})

			_, _ = tx.ExecContext(ctx, "INSERT INTO audit_logs (user_id, action, target_type, target_id, meta, created_at) VALUES ($1,'force_delete','content',$2,$3,now())", auth.GetUserIDFromCtx(c.Request.Context()), body.ContentID, mapToJSON(map[string]string{"blobKey": blobKey}))
		}
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": "userFileId or contentId required"})
		return
	}

	if err := tx.Commit(); err != nil {
		logger.L.Error("force-delete tx commit failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "force delete scheduled"})
}

func nullableToString(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}
