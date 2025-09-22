package rest

import (
	"context"
	"database/sql"
	"net/http"

	"backend/internal/audit"
	"backend/internal/auth"
	"backend/internal/cache"
	fsvc "backend/internal/files"
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

func RegisterFileRoutes(rg *gin.RouterGroup, db *sql.DB, st *storage.MinioStorage, jwtSecret string, cache cache.Cache, publish func(ctx context.Context, fileID string, downloadCount int64) error) {
	files := rg.Group("/files")
	files.Use(auth.RequireAuth(jwtSecret))
	producer := worker.NewProducer()
	svc := fsvc.New(db, st, cache, publish, producer)
	h := &fileHandler{svc: svc, db: db}
	files.GET("", h.listFilesPrimary)
	files.GET("/:id", h.getFileMetadata)
	files.GET("/:id/download", h.download)
	files.DELETE("/:id", h.delete)
	files.POST("/:id/restore", h.restore)
	files.PATCH("/:id", h.patch)
	files.POST("/:id/move", h.move)
	files.POST("/:id/versions", h.createVersion)
	files.GET("/:id/versions", h.listVersions)
}

type fileHandler struct {
	svc fsvc.Service
	db  *sql.DB
}

// Download godoc
//
//	@Summary		Get download URL
//	@Description	Generate a pre-signed download URL for a file
//	@Tags			files
//	@Accept			json
//	@Produce		json
//	@Param			id	path		string				true	"File ID"
//	@Success		200	{object}	downloadURLResponse	"Download URL response"
//	@Failure		403	{object}	errorResponse		"Forbidden"
//	@Failure		404	{object}	errorResponse		"File not found"
//	@Security		BearerAuth
//	@Router			/api/v1/files/{id}/download [get]
func (h *fileHandler) download(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	fileId := c.Param("id")
	url, err := h.svc.GetDownloadURL(c.Request.Context(), userID, fileId)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, errorResponse{Error: "file not found"})
			return
		}
		c.JSON(http.StatusForbidden, errorResponse{Error: err.Error()})
		return
	}
	_ = audit.Log(
		c.Request.Context(),
		h.db,
		userID,
		"download",
		"file",
		fileId,
		map[string]interface{}{
			"fileID": fileId,
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
