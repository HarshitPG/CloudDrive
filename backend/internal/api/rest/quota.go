package rest

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"backend/internal/auth"
	qsvc "backend/internal/quota"

	"github.com/gin-gonic/gin"
)

func RegisterQuotaRoutes(rg *gin.RouterGroup, db *sql.DB, jwtSecret string) {
	users := rg.Group("/users")
	users.Use(auth.RequireAuth(jwtSecret))
	h := &quotaHandler{svc: qsvc.New(db)}
	users.GET("/me/usage", h.getMyUsage)

	admin := users.Group("").Use(auth.AdminOnly(db))
	admin.PATCH("/:id/quota", h.patchQuotaHandler)
}

type quotaHandler struct{ svc qsvc.Service }

func (h *quotaHandler) getMyUsage(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	usage, err := h.svc.GetUsage(ctx, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"original_bytes":     usage.OriginalBytes,
		"deduped_bytes":      usage.DedupedBytes,
		"quota_bytes":        usage.QuotaBytes,
		"savings_bytes":      usage.SavingsBytes,
		"savings_percent":    usage.SavingsPercent,
		"quota_used_percent": usage.QuotaUsedPercent,
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
	if err := h.svc.UpdateQuota(ctx, targetID, body.QuotaBytes); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "quota updated"})
}
