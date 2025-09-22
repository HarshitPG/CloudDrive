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

func (h *fileHandler) download(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	fileId := c.Param("id")
	url, err := h.svc.GetDownloadURL(c.Request.Context(), userID, fileId)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
			return
		}
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
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

	c.JSON(http.StatusOK, gin.H{"downloadUrl": url})
}

func (h *fileHandler) delete(c *gin.Context) {
	ctx := c.Request.Context()
	userID := auth.GetUserIDFromCtx(ctx)
	fileId := c.Param("id")
	permanent := c.Query("permanent") == "true"
	msg, err := h.svc.Delete(ctx, userID, fileId, permanent)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	_ = audit.Log(ctx, h.db, userID, "delete", "file", fileId, map[string]interface{}{"permanent": permanent})
	c.JSON(http.StatusOK, gin.H{"message": msg})
}

func (h *fileHandler) restore(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	fileId := c.Param("id")
	msg, err := h.svc.Restore(c.Request.Context(), userID, fileId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	_ = audit.Log(c.Request.Context(), h.db, userID, "restore", "file", fileId, map[string]interface{}{"fileID": fileId})
	c.JSON(http.StatusOK, gin.H{"message": msg})
}

type patchRequest struct {
	Filename string   `json:"filename"`
	Tags     []string `json:"tags"`
}

func (h *fileHandler) patch(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	fileId := c.Param("id")
	var req patchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.Patch(c.Request.Context(), userID, fileId, fsvc.PatchRequest{Filename: req.Filename, Tags: req.Tags}); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "updated"})
}

type moveRequest struct {
	TargetFolderId string `json:"targetFolderId"`
}

func (h *fileHandler) move(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	fileId := c.Param("id")

	var req moveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.Move(c.Request.Context(), userID, fileId, req.TargetFolderId); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "moved"})
}

func (h *fileHandler) createVersion(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	fileId := c.Param("id")
	var body struct {
		ContentID string `json:"content_id" binding:"required"`
		Filename  string `json:"filename"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.CreateVersion(c.Request.Context(), userID, fileId, fsvc.CreateVersionRequest{ContentID: body.ContentID, Filename: body.Filename}); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "version created"})
}

func (h *fileHandler) listVersions(c *gin.Context) {
	fileId := c.Param("id")
	versions, err := h.svc.ListVersions(c.Request.Context(), fileId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"versions": versions})
}

func (h *fileHandler) getFileMetadata(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	fileId := c.Param("id")
	meta, err := h.svc.GetMetadata(c.Request.Context(), userID, fileId)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, meta)
}

func (h *fileHandler) listFiles(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	folderID := c.Query("folderId")
	deleted := c.Query("deleted") == "true"
	files, err := h.svc.ListFiles(c.Request.Context(), userID, folderID, deleted)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"files": files})
}

// listFilesPrimary returns only primary files: files whose folder is NULL or whose parent folder is not in trash.
// It preserves the existing 'deleted' parameter behavior for listing trash.
func (h *fileHandler) listFilesPrimary(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	folderID := c.Query("folderId")
	deleted := c.Query("deleted") == "true"
	files, err := h.svc.ListFilesPrimary(c.Request.Context(), userID, folderID, deleted)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"files": files})
}
