package rest

import (
	"database/sql"
	"net/http"

	"backend/internal/auth"
	"backend/internal/cache"
	"backend/internal/storage"
	uploadsvc "backend/internal/uploads"
	"backend/internal/worker"

	"github.com/gin-gonic/gin"
)

func RegisterUploadRoutes(rg *gin.RouterGroup, db *sql.DB, st *storage.MinioStorage, jwtSecret string, c cache.Cache) {
	uploads := rg.Group("/uploads")
	uploads.Use(auth.RequireAuth(jwtSecret))
	h := &uploadHandler{
		svc:      uploadsvc.New(db, st, c),
		producer: worker.NewProducer(),
	}

	uploads.POST("/session", h.createSession)
	uploads.POST("/complete", h.complete)
	uploads.POST("/abort", h.abort)
	uploads.POST("/folder/init", h.folderInit)
}

type uploadHandler struct {
	svc      uploadsvc.Service
	producer *worker.Producer
}

type FolderInitFile = uploadsvc.FolderInitFile
type FolderInitRequest struct {
	ParentID       string           `json:"parentId"`
	RootName       string           `json:"rootName" binding:"required"`
	Files          []FolderInitFile `json:"files" binding:"required"`
	IdempotencyKey string           `json:"idempotencyKey"`
}
type FolderInitFileResponse = uploadsvc.FolderInitFileResponse
type FolderInitResponse = uploadsvc.FolderInitResponse

func (h *uploadHandler) folderInit(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	var req FolderInitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	res, err := h.svc.FolderInit(c.Request.Context(), userID, req.ParentID, req.RootName, req.Files)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

type createSessionRequest = uploadsvc.CreateSessionRequest
type createSessionResponse = uploadsvc.CreateSessionResponse

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
	res, err := h.svc.CreateSession(c.Request.Context(), userID, uploadsvc.CreateSessionRequest(req))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !res.SkipUpload {
		c.JSON(http.StatusCreated, res)
		return
	}
	c.JSON(http.StatusOK, res)
}

type completeRequest = uploadsvc.CompleteRequest

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
	res, err := h.svc.Complete(c.Request.Context(), userID, uploadsvc.CompleteRequest(req))
	if err != nil {
		if err.Error() == "not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
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
	if err := h.svc.Abort(c.Request.Context(), userID, req.SessionId); err != nil {
		if err.Error() == "not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "aborted"})
}
