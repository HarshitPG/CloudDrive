package rest

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"backend/internal/auth"
	"backend/pkg/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func RegisterQuotaRoutes(rg *gin.RouterGroup, db *sql.DB, jwtSecret string) {
	users := rg.Group("/users")
	users.Use(auth.RequireAuth(jwtSecret))
	h := &quotaHandler{db: db}
	users.GET("/me/usage", h.getMyUsage)

	admin := users.Group("").Use(auth.AdminOnly(db))

	admin.PATCH("/:id/quota", h.patchQuotaHandler)
}

type quotaHandler struct {
	db *sql.DB
}

func (h *quotaHandler) getMyUsage(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()

	var original int64
	if err := h.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(original_size_bytes),0) FROM user_files
		WHERE user_id=$1
	`, userID).Scan(&original); err != nil {
		logger.L.Error("original usage query failed", zap.Error(err), zap.String("userID", userID))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}

	var deduped int64
	if err := h.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(fc.size_bytes),0)
		FROM file_contents fc
		JOIN (
			SELECT DISTINCT content_id FROM user_files
			WHERE user_id=$1
		) u ON u.content_id = fc.id
	`, userID).Scan(&deduped); err != nil {
		logger.L.Error("deduped usage query failed", zap.Error(err), zap.String("userID", userID))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}

	var quota int64
	if err := h.db.QueryRowContext(ctx, `SELECT quota_bytes FROM users WHERE id=$1`, userID).Scan(&quota); err != nil {
		logger.L.Error("quota lookup failed", zap.Error(err), zap.String("userID", userID))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}

	savingsBytes := original - deduped
	var savingsPct float64
	if original > 0 {
		savingsPct = (float64(savingsBytes) / float64(original)) * 100
	}
	var usedPct float64
	if quota > 0 {
		usedPct = (float64(deduped) / float64(quota)) * 100
	}
	c.JSON(http.StatusOK, gin.H{
		"original_bytes": original,
		"deduped_bytes":  deduped,
		"quota_bytes":    quota,
		"savings_bytes":  savingsBytes,
		"savings_percent": func() float64 {
			if savingsPct < 0 {
				return 0
			}
			return savingsPct
		}(), "quota_used_percent": usedPct,
	})
}

func (h *quotaHandler) patchQuotaHandler(c *gin.Context) {
	targetID := c.Param("id")
	var body struct {
		QuotaBytes int64 `json:"quotaBytes" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if body.QuotaBytes < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "quota must be >= 0"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()

	res, err := h.db.ExecContext(ctx, "UPDATE users SET quota_bytes=$1 WHERE id=$2", body.QuotaBytes, targetID)
	if err != nil {
		logger.L.Error("update quota failed", zap.Error(err), zap.String("targetID", targetID))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "quota updated"})
}
