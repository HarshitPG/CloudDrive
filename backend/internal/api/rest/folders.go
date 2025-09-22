package rest

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"

	"backend/internal/auth"
	"backend/internal/cache"
	folders2 "backend/internal/folders"
	"backend/internal/storage"
	"backend/internal/worker"
	"backend/pkg/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func RegisterFolderRoutes(rg *gin.RouterGroup, db *sql.DB, jwtSecret string, c cache.Cache, st *storage.MinioStorage) {
	folders := rg.Group("/folders")
	folders.Use(auth.RequireAuth(jwtSecret))
	svc := folders2.New(db, c, st, worker.NewProducer())
	h := &folderHandler{db: db, cache: c, producer: worker.NewProducer(), storage: st, svc: svc}

	folders.GET("", h.listPrimary)
	folders.POST("", h.create)
	folders.GET("/:id/contents", h.listContents)
	folders.GET("/:id/files", h.listFilesInFolder)
	folders.GET("/:id/tree", h.getTree)
	folders.GET("/:id/download", h.downloadArchive)
	folders.GET("/:id/ancestors", h.getAncestors)
	folders.PATCH("/:id", h.rename)
	folders.DELETE("/:id", h.delete)
	folders.POST("/:id/move", h.move)
}

type folderHandler struct {
	db       *sql.DB
	cache    cache.Cache
	producer *worker.Producer
	storage  *storage.MinioStorage
	svc      folders2.Service
}

type createFolderRequest struct {
	Name     string `json:"name" binding:"required"`
	ParentID string `json:"parentId"`
}

func (h *folderHandler) list(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	parentID := c.Query("parentId")

	limit := 20
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	offset := 0
	if v := c.Query("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			offset = n
		}
	}
	if offset < 0 {
		offset = 0
	}
	if offset > 100000 {
		offset = 100000
	}

	var (
		rows *sql.Rows
		err  error
	)

	if parentID == "" {
		rows, err = h.db.QueryContext(c.Request.Context(), `
					SELECT id, name, created_at, updated_at,
						COALESCE((SELECT SUM(fc.size_bytes)
											FROM user_files uf
											JOIN file_contents fc ON uf.content_id = fc.id
											WHERE uf.folder_id = folders.id AND uf.deleted_at IS NULL), 0) AS size
			FROM folders
			WHERE user_id=$1 AND parent_id IS NULL AND deleted_at IS NULL
			ORDER BY created_at DESC
			LIMIT $2 OFFSET $3
		`, userID, limit, offset)
	} else {
		rows, err = h.db.QueryContext(c.Request.Context(), `
					SELECT id, name, created_at, updated_at,
						COALESCE((SELECT SUM(fc.size_bytes)
											FROM user_files uf
											JOIN file_contents fc ON uf.content_id = fc.id
											WHERE uf.folder_id = f.id AND uf.deleted_at IS NULL), 0) AS size
			FROM folders
			WHERE user_id=$1 AND parent_id=$2 AND deleted_at IS NULL
			ORDER BY created_at DESC
			LIMIT $3 OFFSET $4
		`, userID, parentID, limit, offset)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}
	defer rows.Close()

	out := make([]map[string]interface{}, 0, limit)
	for rows.Next() {
		var id, name, createdAt, updatedAt string
		var size sql.NullInt64
		if err := rows.Scan(&id, &name, &createdAt, &updatedAt, &size); err != nil {
			continue
		}
		s := int64(0)
		if size.Valid {
			s = size.Int64
		}
		out = append(out, map[string]interface{}{
			"id":        id,
			"name":      name,
			"createdAt": createdAt,
			"updatedAt": updatedAt,
			"size":      s,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"folders": out,
		"page": gin.H{
			"limit":  limit,
			"offset": offset,
		},
	})
}

func (h *folderHandler) listPrimary(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	parentID := c.Query("parentId")
	deleted := c.Query("deleted") == "true"
	limit, offset := 20, 0
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	if v := c.Query("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			offset = n
		}
	}
	res, err := h.svc.ListPrimary(c.Request.Context(), userID, parentID, deleted, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}
	out := make([]map[string]interface{}, 0, len(res.Folders))
	for _, it := range res.Folders {
		item := map[string]interface{}{"id": it.ID, "name": it.Name, "createdAt": it.CreatedAt, "updatedAt": it.UpdatedAt, "size": it.Size}
		if it.DeletedAt != nil {
			item["deletedAt"] = *it.DeletedAt
		}
		out = append(out, item)
	}
	c.JSON(http.StatusOK, gin.H{"folders": out, "page": gin.H{"limit": res.Limit, "offset": res.Offset}})
}

func (h *folderHandler) create(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	var req createFolderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	id, err := h.svc.Create(c.Request.Context(), userID, req.ParentID, req.Name)
	if err != nil {
		logger.L.Error("folders.create: insert failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id, "message": "folder created"})
}

func (h *folderHandler) listContents(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	folderID := c.Param("id")
	res, err := h.svc.ListContents(c.Request.Context(), userID, folderID)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "folder not found"})
			return
		}
		if err.Error() == "forbidden" {
			c.JSON(http.StatusForbidden, gin.H{"error": "not authorized"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}
	subs := make([]map[string]interface{}, 0, len(res.Folders))
	for _, s := range res.Folders {
		subs = append(subs, map[string]interface{}{"id": s.ID, "name": s.Name, "createdAt": s.CreatedAt})
	}
	files := make([]map[string]interface{}, 0, len(res.Files))
	for _, f := range res.Files {
		files = append(files, map[string]interface{}{"id": f.ID, "filename": f.Filename, "mime": f.MIME, "size": f.Size, "createdAt": f.CreatedAt})
	}
	c.JSON(http.StatusOK, gin.H{"folders": subs, "files": files})
}

type renameFolderRequest struct {
	Name string `json:"name" binding:"required"`
}

func (h *folderHandler) rename(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	folderID := c.Param("id")
	var req renameFolderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.Rename(c.Request.Context(), userID, folderID, req.Name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "folder renamed"})
}

func (h *folderHandler) downloadArchive(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	folderID := c.Param("id")
	recursive := c.Query("recursive") == "true"

	var folderName string
	_ = h.db.QueryRowContext(c.Request.Context(), `SELECT name FROM folders WHERE id=$1`, folderID).Scan(&folderName)
	if folderName == "" {
		folderName = "folder"
	}
	zipFilename := folderName + ".zip"
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", zipFilename))
	c.Header("Cache-Control", "no-cache")

	if err := h.svc.WriteArchive(c.Request.Context(), userID, folderID, recursive, c.Writer); err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "no files found in folder"})
			return
		}
		if err.Error() == "forbidden" {
			c.JSON(http.StatusForbidden, gin.H{"error": "not authorized"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
}

type moveFolderRequest struct {
	TargetParentID string `json:"targetParentId"`
}

func (h *folderHandler) move(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	folderID := c.Param("id")
	var req moveFolderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.Move(c.Request.Context(), userID, folderID, req.TargetParentID); err != nil {
		if err.Error() == "cannot move folder inside itself" {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "folder moved"})
}

func (h *folderHandler) listFilesInFolder(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	folderID := c.Param("id")
	limit, offset := 20, 0
	if v := c.Query("limit"); v != "" {
		if n, e := strconv.Atoi(v); e == nil {
			limit = n
		}
	}
	if v := c.Query("offset"); v != "" {
		if n, e := strconv.Atoi(v); e == nil {
			offset = n
		}
	}
	res, err := h.svc.ListFilesInFolder(c.Request.Context(), userID, folderID, limit, offset)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "folder not found"})
			return
		}
		if err.Error() == "forbidden" {
			c.JSON(http.StatusForbidden, gin.H{"error": "not authorized"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}
	files := make([]map[string]interface{}, 0, len(res.Files))
	for _, it := range res.Files {
		files = append(files, map[string]interface{}{
			"id": it.ID, "filename": it.Filename, "mime": it.MIME, "size": it.Size,
			"createdAt": it.CreatedAt, "updatedAt": it.UpdatedAt, "downloadCount": it.DownloadCount,
			"contentHash": it.ContentHash, "physicalSize": it.PhysicalSize, "refCount": it.RefCount, "dedupSavings": it.DedupSavings,
		})
	}
	c.JSON(http.StatusOK, gin.H{"files": files, "page": gin.H{"limit": res.Limit, "offset": res.Offset}})
}

func (h *folderHandler) getTree(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	rootID := c.Param("id")
	tree, err := h.svc.GetTree(c.Request.Context(), userID, rootID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}
	c.JSON(http.StatusOK, tree)
}

func (h *folderHandler) getAncestors(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	folderID := c.Param("id")
	list, err := h.svc.GetAncestors(c.Request.Context(), userID, folderID)
	if err != nil {
		logger.L.Error("folders.getAncestors: query failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}
	out := make([]map[string]string, 0, len(list))
	for _, a := range list {
		out = append(out, map[string]string{"id": a.ID, "name": a.Name})
	}
	c.JSON(http.StatusOK, gin.H{"ancestors": out})
}

func (h *folderHandler) delete(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	folderID := c.Param("id")
	permanent := c.Query("permanent") == "true"
	msg, err := h.svc.Delete(c.Request.Context(), userID, folderID, permanent)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": msg})
}
