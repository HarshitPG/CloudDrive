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

// Admin Response Models
type adminUsageResponse struct {
	Items []asvc.UsageRow `json:"items"`
}

type adminAuditResponse struct {
	Items []map[string]interface{} `json:"items"`
}

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

// AdminUsageHandler godoc
//
//	@Summary		Get system usage statistics (admin only)
//	@Description	Retrieve comprehensive system usage statistics including user storage consumption
//	@Tags			admin
//	@Produce		json
//	@Success		200	{object}	map[string]interface{}	"System usage statistics"
//	@Failure		401	{object}	map[string]string		"Unauthorized"
//	@Failure		403	{object}	map[string]string		"Admin access required"
//	@Failure		500	{object}	map[string]string		"Internal server error"
//	@Security		BearerAuth
//	@Router			/api/v1/admin/usage [get]
func (h *adminHandler) adminUsageHandler(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	rows, err := h.svc.Usage(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "internal"})
		return
	}
	c.JSON(http.StatusOK, adminUsageResponse{Items: rows})
}

// AdminAuditHandler godoc
//
//	@Summary		Get audit log (admin only)
//	@Description	Retrieve system audit log with user actions and system events
//	@Tags			admin
//	@Produce		json
//	@Param			limit	query		int						false	"Maximum number of entries to return (1-1000, default: 100)"
//	@Param			offset	query		int						false	"Number of entries to skip for pagination (default: 0)"
//	@Success		200		{object}	map[string]interface{}	"Audit log entries"
//	@Failure		401		{object}	map[string]string		"Unauthorized"
//	@Failure		403		{object}	map[string]string		"Admin access required"
//	@Failure		500		{object}	map[string]string		"Internal server error"
//	@Security		BearerAuth
//	@Router			/api/v1/admin/audit [get]
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
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "internal"})
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
	c.JSON(http.StatusOK, adminAuditResponse{Items: out})
}

// AdminForceDeleteHandler godoc
//
//	@Summary		Force delete file or content (admin only)
//	@Description	Administratively force deletion of a user file or content, bypassing normal deletion rules
//	@Tags			admin
//	@Accept			json
//	@Produce		json
//	@Param			body	body		map[string]string	true	"Force delete request with userFileId or contentId"
//	@Success		200		{object}	map[string]string	"Force delete scheduled"
//	@Failure		400		{object}	map[string]string	"Bad request"
//	@Failure		401		{object}	map[string]string	"Unauthorized"
//	@Failure		403		{object}	map[string]string	"Admin access required"
//	@Failure		404		{object}	map[string]string	"File not found"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/api/v1/admin/force-delete [post]
func (h *adminHandler) adminForceDeleteHandler(c *gin.Context) {
	var body struct {
		UserFileID string `json:"userFileId"`
		ContentID  string `json:"contentId"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: "invalid body"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	if err := h.svc.ForceDelete(ctx, auth.GetUserIDFromCtx(c.Request.Context()), body.UserFileID, body.ContentID); err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, errorResponse{Error: "not found"})
			return
		}
		c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, messageResponse{Message: "force delete scheduled"})
}
