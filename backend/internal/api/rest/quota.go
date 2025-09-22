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

// Quota Response Models
type quotaUsageResponse struct {
	OriginalBytes    int64   `json:"original_bytes"`
	DedupedBytes     int64   `json:"deduped_bytes"`
	QuotaBytes       int64   `json:"quota_bytes"`
	SavingsBytes     int64   `json:"savings_bytes"`
	SavingsPercent   float64 `json:"savings_percent"`
	QuotaUsedPercent float64 `json:"quota_used_percent"`
}

func RegisterQuotaRoutes(rg *gin.RouterGroup, db *sql.DB, jwtSecret string) {
	users := rg.Group("/users")
	users.Use(auth.RequireAuth(jwtSecret))
	h := &quotaHandler{svc: qsvc.New(db)}
	users.GET("/me/usage", h.getMyUsage)

	admin := users.Group("").Use(auth.AdminOnly(db))
	admin.PATCH("/:id/quota", h.patchQuotaHandler)
}

type quotaHandler struct{ svc qsvc.Service }

// GetMyUsage godoc
//
//	@Summary		Get current user quota usage
//	@Description	Retrieve quota usage statistics for the authenticated user including storage metrics and savings
//	@Tags			quota
//	@Produce		json
//	@Success		200	{object}	quotaUsageResponse	"Quota usage details"
//	@Failure		401	{object}	errorResponse		"Unauthorized"
//	@Failure		500	{object}	errorResponse		"Internal server error"
//	@Security		BearerAuth
//	@Router			/api/v1/users/me/usage [get]
func (h *quotaHandler) getMyUsage(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	if userID == "" {
		c.JSON(http.StatusUnauthorized, errorResponse{Error: "unauthenticated"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	usage, err := h.svc.GetUsage(ctx, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "internal"})
		return
	}
	c.JSON(http.StatusOK, quotaUsageResponse{
		OriginalBytes:    usage.OriginalBytes,
		DedupedBytes:     usage.DedupedBytes,
		QuotaBytes:       usage.QuotaBytes,
		SavingsBytes:     usage.SavingsBytes,
		SavingsPercent:   usage.SavingsPercent,
		QuotaUsedPercent: usage.QuotaUsedPercent,
	})
}

// PatchQuotaHandler godoc
//
//	@Summary		Update user quota (admin only)
//	@Description	Update the storage quota limit for a specific user (requires admin privileges)
//	@Tags			quota
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string				true	"User ID"
//	@Param			body	body		map[string]int64	true	"Quota update request"
//	@Success		200		{object}	messageResponse		"Quota updated successfully"
//	@Failure		400		{object}	errorResponse		"Bad request"
//	@Failure		401		{object}	errorResponse		"Unauthorized"
//	@Failure		403		{object}	errorResponse		"Admin access required"
//	@Failure		500		{object}	errorResponse		"Internal server error"
//	@Security		BearerAuth
//	@Router			/api/v1/users/{id}/quota [patch]
func (h *quotaHandler) patchQuotaHandler(c *gin.Context) {
	targetID := c.Param("id")
	var body struct {
		QuotaBytes int64 `json:"quotaBytes" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: "invalid body"})
		return
	}
	if body.QuotaBytes < 0 {
		c.JSON(http.StatusBadRequest, errorResponse{Error: "quota must be >= 0"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	if err := h.svc.UpdateQuota(ctx, targetID, body.QuotaBytes); err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "internal"})
		return
	}
	c.JSON(http.StatusOK, messageResponse{Message: "quota updated"})
}
