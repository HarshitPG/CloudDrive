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

// Folder Response Models
type folderListResponse struct {
	Folders []folders2.FolderItem `json:"folders"`
	Page    pageInfo              `json:"page"`
}

type folderContentsResponse struct {
	Folders []folders2.FolderChild `json:"folders"`
	Files   []folders2.FileChild   `json:"files"`
}

type folderFilesResponse struct {
	Files []folders2.FileListItem `json:"files"`
	Page  pageInfo                `json:"page"`
}

type ancestorsResponse struct {
	Ancestors []folders2.Ancestor `json:"ancestors"`
}

type pageInfo struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

type createFolderResponse struct {
	ID      string `json:"id" example:"folder_123"`
	Message string `json:"message" example:"folder created"`
}

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
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "db error"})
		return
	}
	defer rows.Close()

	out := make([]folders2.FolderItem, 0, limit)
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
		out = append(out, folders2.FolderItem{
			ID:        id,
			Name:      name,
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
			Size:      s,
		})
	}

	c.JSON(http.StatusOK, folderListResponse{
		Folders: out,
		Page: pageInfo{
			Limit:  limit,
			Offset: offset,
		},
	})
}

// ListPrimary godoc
//
//	@Summary		List folders
//	@Description	List user's folders with optional parent filtering and trash support
//	@Tags			folders
//	@Accept			json
//	@Produce		json
//	@Param			parentId	query		string					false	"Parent folder ID"
//	@Param			deleted		query		boolean					false	"List deleted folders"
//	@Param			limit		query		integer					false	"Limit results (default 20)"
//	@Param			offset		query		integer					false	"Offset for pagination (default 0)"
//	@Success		200			{object}	map[string]interface{}	"Folders list response"
//	@Failure		500			{object}	map[string]string		"Internal server error"
//	@Security		BearerAuth
//	@Router			/api/v1/folders [get]
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
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "db error"})
		return
	}
	c.JSON(http.StatusOK, folderListResponse{
		Folders: res.Folders,
		Page: pageInfo{
			Limit:  res.Limit,
			Offset: res.Offset,
		},
	})
}

// Create godoc
//
//	@Summary		Create folder
//	@Description	Create a new folder under optional parent
//	@Tags			folders
//	@Accept			json
//	@Produce		json
//	@Param			body	body		createFolderRequest		true	"Folder creation payload"
//	@Success		201		{object}	map[string]interface{}	"Created folder response"
//	@Failure		400		{object}	map[string]string		"Bad request"
//	@Failure		500		{object}	map[string]string		"Internal server error"
//	@Security		BearerAuth
//	@Router			/api/v1/folders [post]
func (h *folderHandler) create(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	var req createFolderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	id, err := h.svc.Create(c.Request.Context(), userID, req.ParentID, req.Name)
	if err != nil {
		logger.L.Error("folders.create: insert failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "db error"})
		return
	}
	c.JSON(http.StatusCreated, createFolderResponse{ID: id, Message: "folder created"})
}

// ListContents godoc
//
//	@Summary		List folder contents
//	@Description	Get all files and subfolders within a folder
//	@Tags			folders
//	@Accept			json
//	@Produce		json
//	@Param			id	path		string					true	"Folder ID"
//	@Success		200	{object}	map[string]interface{}	"Folder contents"
//	@Failure		403	{object}	map[string]string		"Forbidden"
//	@Failure		404	{object}	map[string]string		"Folder not found"
//	@Failure		500	{object}	map[string]string		"Internal server error"
//	@Security		BearerAuth
//	@Router			/api/v1/folders/{id}/contents [get]
func (h *folderHandler) listContents(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	folderID := c.Param("id")
	res, err := h.svc.ListContents(c.Request.Context(), userID, folderID)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, errorResponse{Error: "folder not found"})
			return
		}
		if err.Error() == "forbidden" {
			c.JSON(http.StatusForbidden, errorResponse{Error: "not authorized"})
			return
		}
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "db error"})
		return
	}
	c.JSON(http.StatusOK, folderContentsResponse{Folders: res.Folders, Files: res.Files})
}

type renameFolderRequest struct {
	Name string `json:"name" binding:"required"`
}

// Rename godoc
//
//	@Summary		Rename folder
//	@Description	Update the name of a folder
//	@Tags			folders
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string				true	"Folder ID"
//	@Param			body	body		renameFolderRequest	true	"Rename request"
//	@Success		200		{object}	map[string]string	"Rename confirmation"
//	@Failure		400		{object}	map[string]string	"Bad request"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/api/v1/folders/{id} [patch]
func (h *folderHandler) rename(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	folderID := c.Param("id")
	var req renameFolderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	if err := h.svc.Rename(c.Request.Context(), userID, folderID, req.Name); err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "db error"})
		return
	}
	c.JSON(http.StatusOK, messageResponse{Message: "folder renamed"})
}

// DownloadArchive godoc
//
//	@Summary		Download folder as archive
//	@Description	Download a folder and its contents as a ZIP archive
//	@Tags			folders
//	@Produce		application/zip
//	@Param			id	path		string				true	"Folder ID"
//	@Success		200	{file}		file				"ZIP archive"
//	@Failure		400	{object}	map[string]string	"Bad request"
//	@Failure		404	{object}	map[string]string	"Folder not found"
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/api/v1/folders/{id}/download [get]
func (h *folderHandler) downloadArchive(c *gin.Context) {
	//	@Summary	Download folder as zip
	//	@Tags		folders
	//	@Produce	application/zip
	//	@Param		id			path	string	true	"Folder ID"
	//	@Param		recursive	query	boolean	false	"Include subfolders"
	//	@Security	BearerAuth
	//	@Success	200	{file}	file	"ZIP archive"
	//	@Router		/api/v1/folders/{id}/download [get]
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
			c.JSON(http.StatusNotFound, errorResponse{Error: "no files found in folder"})
			return
		}
		if err.Error() == "forbidden" {
			c.JSON(http.StatusForbidden, errorResponse{Error: "not authorized"})
			return
		}
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "internal"})
		return
	}
}

type moveFolderRequest struct {
	TargetParentID string `json:"targetParentId"`
}

// Move godoc
//
//	@Summary		Move folder
//	@Description	Move a folder to a different parent folder
//	@Tags			folders
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string					true	"Folder ID"
//	@Param			body	body		moveFolderRequest		true	"Move request"
//	@Success		200		{object}	map[string]interface{}	"Updated folder information"
//	@Failure		400		{object}	map[string]string		"Bad request"
//	@Failure		404		{object}	map[string]string		"Folder not found"
//	@Failure		500		{object}	map[string]string		"Internal server error"
//	@Security		BearerAuth
//	@Router			/api/v1/folders/{id}/move [patch]
func (h *folderHandler) move(c *gin.Context) {
	//	@Summary	Move folder
	//	@Tags		folders
	//	@Accept		json
	//	@Produce	json
	//	@Param		id		path	string				true	"Folder ID"
	//	@Param		body	body	moveFolderRequest	true	"Move"
	//	@Security	BearerAuth
	//	@Success	200	{object}	map[string]string
	//	@Router		/api/v1/folders/{id}/move [post]
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	folderID := c.Param("id")
	var req moveFolderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	if err := h.svc.Move(c.Request.Context(), userID, folderID, req.TargetParentID); err != nil {
		if err.Error() == "cannot move folder inside itself" {
			c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "db error"})
		return
	}
	c.JSON(http.StatusOK, messageResponse{Message: "folder moved"})
}

// ListFilesInFolder godoc
//
//	@Summary		List files in folder
//	@Description	Retrieve all files contained within a specific folder
//	@Tags			folders
//	@Produce		json
//	@Param			id		path		string					true	"Folder ID"
//	@Param			page	query		int						false	"Page number (default: 1)"
//	@Param			limit	query		int						false	"Items per page (default: 20)"
//	@Param			sort	query		string					false	"Sort field (name, created_at, updated_at)"
//	@Param			order	query		string					false	"Sort order (asc, desc)"
//	@Success		200		{array}		map[string]interface{}	"Array of files"
//	@Failure		400		{object}	map[string]string		"Bad request"
//	@Failure		404		{object}	map[string]string		"Folder not found"
//	@Failure		500		{object}	map[string]string		"Internal server error"
//	@Security		BearerAuth
//	@Router			/api/v1/folders/{id}/files [get]
func (h *folderHandler) listFilesInFolder(c *gin.Context) {
	//	@Summary	List files in folder
	//	@Tags		folders
	//	@Produce	json
	//	@Param		id		path	string	true	"Folder ID"
	//	@Param		limit	query	integer	false	"Limit"
	//	@Param		offset	query	integer	false	"Offset"
	//	@Security	BearerAuth
	//	@Success	200	{object}	map[string]interface{}
	//	@Router		/api/v1/folders/{id}/files [get]
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
			c.JSON(http.StatusNotFound, errorResponse{Error: "folder not found"})
			return
		}
		if err.Error() == "forbidden" {
			c.JSON(http.StatusForbidden, errorResponse{Error: "not authorized"})
			return
		}
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "db error"})
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
	c.JSON(http.StatusOK, folderFilesResponse{
		Files: res.Files,
		Page: pageInfo{
			Limit:  res.Limit,
			Offset: res.Offset,
		},
	})
}

// GetTree godoc
//
//	@Summary		Get folder tree structure
//	@Description	Retrieve the hierarchical tree structure of a folder and its subfolders
//	@Tags			folders
//	@Produce		json
//	@Param			id		path		string					true	"Folder ID"
//	@Param			depth	query		int						false	"Maximum depth to traverse (default: unlimited)"
//	@Success		200		{object}	map[string]interface{}	"Folder tree structure"
//	@Failure		400		{object}	map[string]string		"Bad request"
//	@Failure		404		{object}	map[string]string		"Folder not found"
//	@Failure		500		{object}	map[string]string		"Internal server error"
//	@Security		BearerAuth
//	@Router			/api/v1/folders/{id}/tree [get]
func (h *folderHandler) getTree(c *gin.Context) {
	//	@Summary	Get folder tree
	//	@Tags		folders
	//	@Produce	json
	//	@Param		id	path	string	true	"Folder ID"
	//	@Security	BearerAuth
	//	@Success	200	{object}	map[string]interface{}
	//	@Router		/api/v1/folders/{id}/tree [get]
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	rootID := c.Param("id")
	tree, err := h.svc.GetTree(c.Request.Context(), userID, rootID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "db error"})
		return
	}
	c.JSON(http.StatusOK, tree)
}

// GetAncestors godoc
//
//	@Summary		Get folder ancestors
//	@Description	Retrieve the path of ancestor folders from root to the specified folder
//	@Tags			folders
//	@Produce		json
//	@Param			id	path		string					true	"Folder ID"
//	@Success		200	{array}		map[string]interface{}	"Array of ancestor folders"
//	@Failure		400	{object}	map[string]string		"Bad request"
//	@Failure		404	{object}	map[string]string		"Folder not found"
//	@Failure		500	{object}	map[string]string		"Internal server error"
//	@Security		BearerAuth
//	@Router			/api/v1/folders/{id}/ancestors [get]
func (h *folderHandler) getAncestors(c *gin.Context) {
	//	@Summary	Get folder ancestors
	//	@Tags		folders
	//	@Produce	json
	//	@Param		id	path	string	true	"Folder ID"
	//	@Security	BearerAuth
	//	@Success	200	{object}	map[string]interface{}
	//	@Router		/api/v1/folders/{id}/ancestors [get]
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	folderID := c.Param("id")
	list, err := h.svc.GetAncestors(c.Request.Context(), userID, folderID)
	if err != nil {
		logger.L.Error("folders.getAncestors: query failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "db error"})
		return
	}
	c.JSON(http.StatusOK, ancestorsResponse{Ancestors: list})
}

// Delete godoc
//
//	@Summary		Delete folder
//	@Description	Permanently delete a folder and all its contents (files and subfolders)
//	@Tags			folders
//	@Produce		json
//	@Param			id	path		string				true	"Folder ID"
//	@Success		200	{object}	map[string]string	"Delete confirmation"
//	@Failure		400	{object}	map[string]string	"Bad request"
//	@Failure		404	{object}	map[string]string	"Folder not found"
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/api/v1/folders/{id} [delete]
func (h *folderHandler) delete(c *gin.Context) {
	//	@Summary	Delete folder
	//	@Tags		folders
	//	@Produce	json
	//	@Param		id			path	string	true	"Folder ID"
	//	@Param		permanent	query	boolean	false	"Permanent delete"
	//	@Security	BearerAuth
	//	@Success	200	{object}	map[string]string
	//	@Router		/api/v1/folders/{id} [delete]
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	folderID := c.Param("id")
	permanent := c.Query("permanent") == "true"
	msg, err := h.svc.Delete(c.Request.Context(), userID, folderID, permanent)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "internal"})
		return
	}
	c.JSON(http.StatusOK, messageResponse{Message: msg})
}
