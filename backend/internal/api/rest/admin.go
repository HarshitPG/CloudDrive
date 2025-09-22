package rest

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"time"

	asvc "backend/internal/admin"
	"backend/internal/auth"
	"backend/internal/worker"

	"github.com/gin-gonic/gin"
)

func RegisterAdminRoutes(rg *gin.RouterGroup, db *sql.DB, jwtSecret string) {
	admin := rg.Group("/admin")
	admin.Use(auth.RequireAuth(jwtSecret))
	h := &adminHandler{svc: asvc.New(db, worker.NewProducer())}
	admin.GET("/usage", h.adminUsageHandler)
	admin.GET("/audit", h.adminAuditHandler)
	admin.POST("/force-delete", h.adminForceDeleteHandler)
}

type adminHandler struct {
	svc asvc.Service
}

func (h *adminHandler) adminUsageHandler(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	rows, err := h.svc.Usage(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows})
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
	items, err := h.svc.Audit(ctx, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	out := []map[string]interface{}{}
	for _, it := range items {
		row := map[string]interface{}{
			"id":        it.ID,
			"createdAt": it.CreatedAt.Format(time.RFC3339),
		}
		if it.UserID != nil {
			row["userId"] = *it.UserID
		}
		if it.Action != nil {
			row["action"] = *it.Action
		}
		if it.TargetType != nil {
			row["targetType"] = *it.TargetType
		}
		if it.TargetID != nil {
			row["targetId"] = *it.TargetID
		}
		if it.Meta != nil {
			row["meta"] = *it.Meta
		}
		out = append(out, row)
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
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
	if err := h.svc.ForceDelete(ctx, auth.GetUserIDFromCtx(c.Request.Context()), body.UserFileID, body.ContentID); err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "force delete scheduled"})
}
