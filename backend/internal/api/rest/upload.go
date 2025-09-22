package rest

import (
	"database/sql"
	"errors"
	"net/http"

	"backend/internal/auth"
	"backend/internal/cache"
	"backend/internal/storage"
	uploadsvc "backend/internal/uploads"
	"backend/internal/worker"

	"github.com/gin-gonic/gin"
)

// Upload Response Models
type abortResponse struct {
	Message string `json:"message" example:"aborted"`
}

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

// FolderInit godoc
//
//	@Summary		Initialize folder upload
//	@Description	Initialize upload session for a folder structure with multiple files
//	@Tags			uploads
//	@Accept			json
//	@Produce		json
//	@Param			body	body		FolderInitRequest	true	"Folder initialization request"
//	@Success		200		{object}	FolderInitResponse	"Upload session initialized"
//	@Failure		400		{object}	map[string]string	"Bad request"
//	@Failure		401		{object}	map[string]string	"Unauthorized"
//	@Failure		413		{object}	map[string]string	"Quota exceeded"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/api/v1/uploads/folder/init [post]
func (h *uploadHandler) folderInit(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	if userID == "" {
		c.JSON(http.StatusUnauthorized, errorResponse{Error: "unauthenticated"})
		return
	}
	var req FolderInitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	res, err := h.svc.FolderInit(c.Request.Context(), userID, req.ParentID, req.RootName, req.Files)
	if err != nil {
		if errors.Is(err, uploadsvc.ErrQuotaExceeded) {
			c.JSON(http.StatusRequestEntityTooLarge, errorResponse{Error: "quota exceeded"})
			return
		}
		c.JSON(http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

type createSessionRequest = uploadsvc.CreateSessionRequest
type createSessionResponse = uploadsvc.CreateSessionResponse

// CreateSession godoc
//
//	@Summary		Create upload session
//	@Description	Create a new file upload session with presigned URLs for multipart upload
//	@Tags			uploads
//	@Accept			json
//	@Produce		json
//	@Param			body	body		createSessionRequest	true	"Upload session request"
//	@Success		201		{object}	createSessionResponse	"Upload session created"
//	@Success		200		{object}	createSessionResponse	"File already exists (skip upload)"
//	@Failure		400		{object}	map[string]string		"Bad request"
//	@Failure		401		{object}	map[string]string		"Unauthorized"
//	@Failure		413		{object}	map[string]string		"Quota exceeded"
//	@Failure		500		{object}	map[string]string		"Internal server error"
//	@Security		BearerAuth
//	@Router			/api/v1/uploads/session [post]
func (h *uploadHandler) createSession(c *gin.Context) {
	var req createSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	if userID == "" {
		c.JSON(http.StatusUnauthorized, errorResponse{Error: "unauthenticated"})
		return
	}
	res, err := h.svc.CreateSession(c.Request.Context(), userID, uploadsvc.CreateSessionRequest(req))
	if err != nil {
		if errors.Is(err, uploadsvc.ErrQuotaExceeded) {
			c.JSON(http.StatusRequestEntityTooLarge, errorResponse{Error: "quota exceeded"})
			return
		}
		c.JSON(http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}
	if !res.SkipUpload {
		c.JSON(http.StatusCreated, res)
		return
	}
	c.JSON(http.StatusOK, res)
}

type completeRequest = uploadsvc.CompleteRequest

// Complete godoc
//
//	@Summary		Complete file upload
//	@Description	Complete a multipart upload session and finalize the file
//	@Tags			uploads
//	@Accept			json
//	@Produce		json
//	@Param			body	body		completeRequest			true	"Upload completion request"
//	@Success		200		{object}	map[string]interface{}	"Upload completed successfully"
//	@Failure		400		{object}	map[string]string		"Bad request"
//	@Failure		401		{object}	map[string]string		"Unauthorized"
//	@Failure		404		{object}	map[string]string		"Session not found"
//	@Failure		413		{object}	map[string]string		"Quota exceeded"
//	@Failure		500		{object}	map[string]string		"Internal server error"
//	@Security		BearerAuth
//	@Router			/api/v1/uploads/complete [post]
func (h *uploadHandler) complete(c *gin.Context) {
	var req completeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	if userID == "" {
		c.JSON(http.StatusUnauthorized, errorResponse{Error: "unauthenticated"})
		return
	}
	res, err := h.svc.Complete(c.Request.Context(), userID, uploadsvc.CompleteRequest(req))
	if err != nil {
		if err.Error() == "not found" {
			c.JSON(http.StatusNotFound, errorResponse{Error: "session not found"})
			return
		}
		if errors.Is(err, uploadsvc.ErrQuotaExceeded) {
			c.JSON(http.StatusRequestEntityTooLarge, errorResponse{Error: "quota exceeded"})
			return
		}
		c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

type abortRequest struct {
	SessionId string `json:"sessionId" binding:"required"`
}

// Abort godoc
//
//	@Summary		Abort file upload
//	@Description	Cancel an ongoing upload session and clean up any partial uploads
//	@Tags			uploads
//	@Accept			json
//	@Produce		json
//	@Param			body	body		abortRequest		true	"Upload abort request"
//	@Success		200		{object}	abortResponse		"Upload aborted successfully"
//	@Failure		400		{object}	map[string]string	"Bad request"
//	@Failure		401		{object}	map[string]string	"Unauthorized"
//	@Failure		404		{object}	map[string]string	"Session not found"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/api/v1/uploads/abort [post]
func (h *uploadHandler) abort(c *gin.Context) {
	var req abortRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	if userID == "" {
		c.JSON(http.StatusUnauthorized, errorResponse{Error: "unauthenticated"})
		return
	}
	if err := h.svc.Abort(c.Request.Context(), userID, req.SessionId); err != nil {
		if err.Error() == "not found" {
			c.JSON(http.StatusNotFound, errorResponse{Error: "session not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, abortResponse{Message: "aborted"})
}
