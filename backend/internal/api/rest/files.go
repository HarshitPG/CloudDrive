package rest

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"backend/internal/audit"
	"backend/internal/auth"
	"backend/internal/cache"
	fsvc "backend/internal/files"
	"backend/internal/llmclient"
	"backend/internal/storage"
	"backend/internal/worker"

	"github.com/gin-gonic/gin"
	//"github.com/google/uuid"
)

// File Response Models
type downloadURLResponse struct {
	DownloadURL string `json:"downloadUrl" example:"https://storage.example.com/files/abc123.pdf"`
}

type fileListResponse struct {
	Files []fsvc.FileListItem `json:"files"`
}

type fileVersionsResponse struct {
	Versions []fsvc.FileVersion `json:"versions"`
}

func RegisterFileRoutes(rg *gin.RouterGroup, db *sql.DB, st *storage.MinioStorage, jwtSecret string, cache cache.Cache, publish func(ctx context.Context, fileID string, downloadCount int64) error, llmClient *llmclient.Client) {
	files := rg.Group("/files")
	files.Use(auth.RequireAuth(jwtSecret))
	producer := worker.NewProducer()
	svc := fsvc.New(db, st, cache, publish, producer)

	var llmSvc llmclient.Service
	if llmClient != nil {
		llmSvc = llmclient.New(llmClient, st)
		llmSvc.StartCleanupRoutine(10*time.Minute, 2*time.Hour)
	}
	h := &fileHandler{svc: svc, db: db, llmSvc: llmSvc}
	files.GET("", h.listFilesPrimary)
	files.GET("/:id", h.getFileMetadata)
	files.GET("/:id/download", h.download)
	files.DELETE("/:id", h.delete)
	files.POST("/:id/restore", h.restore)
	files.PATCH("/:id", h.patch)
	files.POST("/:id/move", h.move)
	files.POST("/:id/versions", h.createVersion)
	files.GET("/:id/versions", h.listVersions)

	files.GET("/:id/summary", h.getSummary)
	files.POST("/:id/process", h.processForChat)
	files.GET("/:id/chat/status", h.getChatStatus)
	files.POST("/:id/chat", h.chat)
	files.GET("/:id/chat/history", h.getChatHistory)
	files.DELETE("/:id/chat/history", h.clearChatHistory)
}

type fileHandler struct {
	svc    fsvc.Service
	db     *sql.DB
	llmSvc llmclient.Service
}

// Download godoc
//
//	@Summary		Download a file
//	@Description	Get a presigned URL to download a file by ID
//	@Tags			files
//	@Produce		json
//	@Param			id	path		string				true	"File ID"
//	@Success		200	{object}	downloadURLResponse	"Presigned URL"
//	@Failure		403	{object}	errorResponse		"Forbidden"
//	@Failure		404	{object}	errorResponse		"File not found"
//	@Security		BearerAuth
//	@Router			/api/v1/files/{id}/download [get]
func (h *fileHandler) download(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	fileID := c.Param("id")
	url, err := h.svc.GetDownloadURL(c.Request.Context(), userID, fileID)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, errorResponse{Error: "file not found"})
			return
		}
		c.JSON(http.StatusForbidden, errorResponse{Error: err.Error()})
		return
	}

	// Trigger async summary generation if LLM service is available
	if h.llmSvc != nil {
		// Query for blob_key and filename
		var blobKey, filename string
		query := `
			SELECT fc.blob_key, uf.filename
			FROM user_files uf
			JOIN file_contents fc ON uf.content_id = fc.id
			WHERE uf.id = $1 AND uf.deleted_at IS NULL
		`
		if err := h.db.QueryRowContext(c.Request.Context(), query, fileID).Scan(&blobKey, &filename); err == nil {
			go h.llmSvc.TriggerSummary(
				context.Background(),
				fileID,
				blobKey,
				filename,
			)
		}
	}
	_ = audit.Log(
		c.Request.Context(),
		h.db,
		userID,
		"download",
		"file",
		fileID,
		map[string]interface{}{
			"fileID": fileID,
		},
	)

	c.JSON(http.StatusOK, downloadURLResponse{DownloadURL: url})
}

// Delete godoc
//
//	@Summary		Delete file
//	@Description	Move file to trash or permanently delete it
//	@Tags			files
//	@Accept			json
//	@Produce		json
//	@Param			id			path		string			true	"File ID"
//	@Param			permanent	query		boolean			false	"Permanent delete (true) or move to trash (false)"
//	@Success		200			{object}	messageResponse	"Delete confirmation"
//	@Failure		400			{object}	errorResponse	"Bad request"
//	@Security		BearerAuth
//	@Router			/api/v1/files/{id} [delete]
func (h *fileHandler) delete(c *gin.Context) {
	ctx := c.Request.Context()
	userID := auth.GetUserIDFromCtx(ctx)
	fileId := c.Param("id")
	permanent := c.Query("permanent") == "true"
	msg, err := h.svc.Delete(ctx, userID, fileId, permanent)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	_ = audit.Log(ctx, h.db, userID, "delete", "file", fileId, map[string]interface{}{"permanent": permanent})
	c.JSON(http.StatusOK, messageResponse{Message: msg})
}

// Restore godoc
//
//	@Summary		Restore file from trash
//	@Description	Restore a deleted file from trash
//	@Tags			files
//	@Accept			json
//	@Produce		json
//	@Param			id	path		string			true	"File ID"
//	@Success		200	{object}	messageResponse	"Restore confirmation"
//	@Failure		400	{object}	errorResponse	"Bad request"
//	@Security		BearerAuth
//	@Router			/api/v1/files/{id}/restore [post]
func (h *fileHandler) restore(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	fileId := c.Param("id")
	msg, err := h.svc.Restore(c.Request.Context(), userID, fileId)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	_ = audit.Log(c.Request.Context(), h.db, userID, "restore", "file", fileId, map[string]interface{}{"fileID": fileId})
	c.JSON(http.StatusOK, messageResponse{Message: msg})
}

type patchRequest struct {
	Filename string   `json:"filename"`
	Tags     []string `json:"tags"`
}

// Patch godoc
//
//	@Summary		Update file metadata
//	@Description	Update file name, tags, or other metadata
//	@Tags			files
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string			true	"File ID"
//	@Param			body	body		patchRequest	true	"File update payload"
//	@Success		200		{object}	messageResponse	"Update confirmation"
//	@Failure		400		{object}	errorResponse	"Bad request"
//	@Security		BearerAuth
//	@Router			/api/v1/files/{id} [patch]
func (h *fileHandler) patch(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	fileId := c.Param("id")
	var req patchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	if err := h.svc.Patch(c.Request.Context(), userID, fileId, fsvc.PatchRequest{Filename: req.Filename, Tags: req.Tags}); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, messageResponse{Message: "updated"})
}

type moveRequest struct {
	TargetFolderId string `json:"targetFolderId"`
}

// Move godoc
//
//	@Summary		Move file to different folder
//	@Description	Move a file to a different folder
//	@Tags			files
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string			true	"File ID"
//	@Param			body	body		moveRequest		true	"Move request payload"
//	@Success		200		{object}	messageResponse	"Move confirmation"
//	@Failure		400		{object}	errorResponse	"Bad request"
//	@Security		BearerAuth
//	@Router			/api/v1/files/{id}/move [post]
func (h *fileHandler) move(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	fileId := c.Param("id")

	var req moveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	if err := h.svc.Move(c.Request.Context(), userID, fileId, req.TargetFolderId); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, messageResponse{Message: "moved"})
}

// CreateVersion godoc
//
//	@Summary		Create new file version
//	@Description	Create a new version of an existing file
//	@Tags			files
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string				true	"File ID"
//	@Param			body	body		object				true	"Version creation payload"
//	@Success		201		{object}	map[string]string	"Version created"
//	@Failure		400		{object}	errorResponse		"Bad request"
//	@Security		BearerAuth
//	@Router			/api/v1/files/{id}/versions [post]
func (h *fileHandler) createVersion(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	fileId := c.Param("id")
	var body struct {
		ContentID string `json:"content_id" binding:"required"`
		Filename  string `json:"filename"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	if err := h.svc.CreateVersion(c.Request.Context(), userID, fileId, fsvc.CreateVersionRequest{ContentID: body.ContentID, Filename: body.Filename}); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusCreated, messageResponse{Message: "version created"})
}

// ListVersions godoc
//
//	@Summary		List file versions
//	@Description	Get all versions of a file
//	@Tags			files
//	@Accept			json
//	@Produce		json
//	@Param			id	path		string					true	"File ID"
//	@Success		200	{object}	fileVersionsResponse	"File versions list"
//	@Failure		500	{object}	map[string]string		"Internal server error"
//	@Security		BearerAuth
//	@Router			/api/v1/files/{id}/versions [get]
func (h *fileHandler) listVersions(c *gin.Context) {
	fileId := c.Param("id")
	versions, err := h.svc.ListVersions(c.Request.Context(), fileId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "internal"})
		return
	}
	c.JSON(http.StatusOK, fileVersionsResponse{Versions: versions})
}

// GetFileMetadata godoc
//
//	@Summary		Get file metadata
//	@Description	Get detailed metadata for a specific file
//	@Tags			files
//	@Accept			json
//	@Produce		json
//	@Param			id	path		string				true	"File ID"
//	@Success		200	{object}	fsvc.FileMetadata	"File metadata"
//	@Failure		404	{object}	map[string]string	"File not found"
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/api/v1/files/{id} [get]
func (h *fileHandler) getFileMetadata(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	fileId := c.Param("id")
	meta, err := h.svc.GetMetadata(c.Request.Context(), userID, fileId)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, errorResponse{Error: "file not found"})
		} else {
			c.JSON(http.StatusInternalServerError, errorResponse{Error: err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, meta)
}

func (h *fileHandler) listFiles(c *gin.Context) {
	//	@Summary	List files (all)
	//	@Tags		files
	//	@Produce	json
	//	@Param		folderId	query		string	false	"Filter by folder"
	//	@Param		deleted		query		boolean	false	"List deleted/trash"
	//	@Success	200			{object}	map[string]interface{}
	//	@Security	BearerAuth
	//	@Router		/api/v1/files [get]
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	folderID := c.Query("folderId")
	deleted := c.Query("deleted") == "true"
	files, err := h.svc.ListFiles(c.Request.Context(), userID, folderID, deleted)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "db error"})
		return
	}
	c.JSON(http.StatusOK, fileListResponse{Files: files})
}

// ListFilesPrimary godoc
//
//	@Summary		List files
//	@Description	List files with optional folder filtering and trash support
//	@Tags			files
//	@Accept			json
//	@Produce		json
//	@Param			folderId	query		string				false	"Filter by folder ID"
//	@Param			deleted		query		boolean				false	"List deleted/trash files"
//	@Success		200			{object}	fileListResponse	"Files list response"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/api/v1/files [get]
func (h *fileHandler) listFilesPrimary(c *gin.Context) {
	//	@Summary	List files
	//	@Tags		files
	//	@Produce	json
	//	@Param		folderId	query		string	false	"Filter by folder"
	//	@Param		deleted		query		boolean	false	"List deleted/trash"
	//	@Success	200			{object}	map[string]interface{}
	//	@Security	BearerAuth
	//	@Router		/api/v1/files [get]
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	folderID := c.Query("folderId")
	deleted := c.Query("deleted") == "true"
	files, err := h.svc.ListFilesPrimary(c.Request.Context(), userID, folderID, deleted)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "db error"})
		return
	}
	c.JSON(http.StatusOK, fileListResponse{Files: files})
}

// clearChatHistory clears history
// @Summary Clear chat history for a file
// @Description Delete all chat messages and history for a file
// @Tags files,llm
// @Produce json
// @Param id path string true "File ID"
// @Success 200 {object} messageResponse
// @Failure 404 {object} errorResponse "File not found"
// @Failure 500 {object} errorResponse "Internal server error"
// @Failure 503 {object} errorResponse "Service unavailable"
// @Security BearerAuth
// @Router /files/{id}/chat/history [delete]
func (h *fileHandler) clearChatHistory(c *gin.Context) {
	if h.llmSvc == nil {
		c.JSON(http.StatusServiceUnavailable, errorResponse{Error: "Chat service unavailable"})
		return
	}

	fileID := c.Param("id")
	err := h.llmSvc.ClearChatHistory(c.Request.Context(), fileID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, messageResponse{Message: "Chat history cleared"})
}

// getChatHistory returns chat history
// @Summary Get chat history for a file
// @Description Retrieve the conversation history for a file's chat
// @Tags files,llm
// @Produce json
// @Param id path string true "File ID"
// @Success 200 {object} object
// @Failure 404 {object} errorResponse "File not found"
// @Failure 500 {object} errorResponse "Internal server error"
// @Failure 503 {object} errorResponse "Service unavailable"
// @Security BearerAuth
// @Router /files/{id}/chat/history [get]
func (h *fileHandler) getChatHistory(c *gin.Context) {
	if h.llmSvc == nil {
		c.JSON(http.StatusServiceUnavailable, errorResponse{Error: "Chat service unavailable"})
		return
	}

	fileID := c.Param("id")
	history, err := h.llmSvc.GetChatHistory(c.Request.Context(), fileID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, history)
}

// chat handles Q&A
// @Summary Ask a question about a file
// @Description Send a question to the LLM about the content of a processed file
// @Tags files,llm
// @Accept json
// @Produce json
// @Param id path string true "File ID"
// @Param request body object true "Question request" SchemaExample({"question": "What is this document about?"})
// @Success 200 {object} object
// @Failure 400 {object} errorResponse "Bad request"
// @Failure 404 {object} errorResponse "File not found"
// @Failure 500 {object} errorResponse "Internal server error"
// @Failure 503 {object} errorResponse "Service unavailable"
// @Security BearerAuth
// @Router /files/{id}/chat [post]
func (h *fileHandler) chat(c *gin.Context) {
	if h.llmSvc == nil {
		c.JSON(http.StatusServiceUnavailable, errorResponse{Error: "Chat service unavailable"})
		return
	}

	fileID := c.Param("id")

	var req struct {
		Question string `json:"question" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: "Invalid request"})
		return
	}

	response, err := h.llmSvc.Chat(c.Request.Context(), fileID, req.Question)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, response)
}

// getChatStatus returns chat readiness
// @Summary Get chat processing status for a file
// @Description Check if a file is ready for chat interactions
// @Tags files,llm
// @Produce json
// @Param id path string true "File ID"
// @Success 200 {object} object
// @Failure 404 {object} errorResponse "File not found"
// @Failure 500 {object} errorResponse "Internal server error"
// @Failure 503 {object} errorResponse "Service unavailable"
// @Security BearerAuth
// @Router /files/{id}/chat/status [get]
func (h *fileHandler) getChatStatus(c *gin.Context) {
	if h.llmSvc == nil {
		c.JSON(http.StatusServiceUnavailable, errorResponse{Error: "Chat service unavailable"})
		return
	}

	fileID := c.Param("id")
	result, err := h.llmSvc.GetChatStatus(c.Request.Context(), fileID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// processForChat starts document indexing
// @Summary Process a file for chat functionality
// @Description Start processing a document to enable Q&A chat capabilities
// @Tags files,llm
// @Produce json
// @Param id path string true "File ID"
// @Success 202 {object} map[string]interface{} "Processing started"
// @Failure 404 {object} errorResponse "File not found"
// @Failure 500 {object} errorResponse "Internal server error"
// @Failure 503 {object} errorResponse "Service unavailable"
// @Security BearerAuth
// @Router /files/{id}/process [post]
func (h *fileHandler) processForChat(c *gin.Context) {
	if h.llmSvc == nil {
		c.JSON(http.StatusServiceUnavailable, errorResponse{Error: "Chat service unavailable"})
		return
	}

	userID := auth.GetUserIDFromCtx(c.Request.Context())
	fileID := c.Param("id")

	var blobKey, filename string
	query := `
        SELECT fc.blob_key, uf.filename
        FROM user_files uf
        JOIN file_contents fc ON uf.content_id = fc.id
        WHERE uf.id = $1 AND uf.user_id = $2 AND uf.deleted_at IS NULL
    `
	if err := h.db.QueryRowContext(c.Request.Context(), query, fileID, userID).Scan(&blobKey, &filename); err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, errorResponse{Error: "file not found"})
		} else {
			c.JSON(http.StatusInternalServerError, errorResponse{Error: err.Error()})
		}
		return
	}

	err := h.llmSvc.ProcessForChat(
		c.Request.Context(),
		fileID,
		blobKey,
		filename,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"message": "Document processing started",
		"file_id": fileID,
	})
}

// getSummary returns summary status
// @Summary Get or generate a quick summary for a file
// @Description Retrieve the summary of a file, generating it if not available
// @Tags files,llm
// @Produce json
// @Param id path string true "File ID"
// @Success 200 {object} object
// @Failure 404 {object} errorResponse "File not found"
// @Failure 500 {object} errorResponse "Internal server error"
// @Failure 503 {object} errorResponse "Service unavailable"
// @Security BearerAuth
// @Router /files/{id}/summary [get]
func (h *fileHandler) getSummary(c *gin.Context) {
	if h.llmSvc == nil {
		c.JSON(http.StatusServiceUnavailable, errorResponse{Error: "Summary service unavailable"})
		return
	}

	fileID := c.Param("id")
	result, err := h.llmSvc.GetSummary(c.Request.Context(), fileID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}
