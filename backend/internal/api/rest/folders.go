package rest

import (
	"database/sql"
	"net/http"

	"backend/internal/auth"

	"github.com/gin-gonic/gin"
)

func RegisterFolderRoutes(rg *gin.RouterGroup, db *sql.DB, jwtSecret string) {
	folders := rg.Group("/folders")
	folders.Use(auth.RequireAuth(jwtSecret))
	h := &folderHandler{db: db}

	folders.POST("", h.create)
	folders.GET("/:id/contents", h.listContents)
	folders.GET("/:id/tree", h.getTree)
	folders.PATCH("/:id", h.rename)
	folders.DELETE("/:id", h.delete)
	folders.POST("/:id/move", h.move)
}

type folderHandler struct {
	db *sql.DB
}

type createFolderRequest struct {
	Name     string `json:"name" binding:"required"`
	ParentID string `json:"parentId"`
}

func (h *folderHandler) create(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	var req createFolderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	_, err := h.db.ExecContext(c.Request.Context(),
		`INSERT INTO folders (id, user_id, parent_id, name, created_at, updated_at)
         VALUES (gen_random_uuid(), $1, NULLIF($2,'')::uuid, $3, now(), now())`,
		userID, req.ParentID, req.Name,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "folder created"})
}

func (h *folderHandler) listContents(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	folderID := c.Param("id")

	folderRows, err := h.db.QueryContext(c.Request.Context(),
		`SELECT id, name, created_at FROM folders 
         WHERE user_id=$1 AND parent_id=$2 AND deleted_at IS NULL`,
		userID, folderID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}
	defer folderRows.Close()

	subfolders := []map[string]interface{}{}
	for folderRows.Next() {
		var id, name, createdAt string
		_ = folderRows.Scan(&id, &name, &createdAt)
		subfolders = append(subfolders, map[string]interface{}{
			"id":         id,
			"name":       name,
			"created_at": createdAt,
		})
	}

	fileRows, err := h.db.QueryContext(c.Request.Context(),
		`SELECT id, filename, declared_mime, original_size_bytes, created_at
         FROM user_files
         WHERE user_id=$1 AND folder_id=$2 AND deleted_at IS NULL`,
		userID, folderID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}
	defer fileRows.Close()

	files := []map[string]interface{}{}
	for fileRows.Next() {
		var id, filename, mime string
		var size int64
		var createdAt string
		_ = fileRows.Scan(&id, &filename, &mime, &size, &createdAt)
		files = append(files, map[string]interface{}{
			"id":        id,
			"filename":  filename,
			"mime":      mime,
			"size":      size,
			"createdAt": createdAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"folders": subfolders,
		"files":   files,
	})
}

func (h *folderHandler) getTree(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	rootID := c.Param("id")

	tree, err := h.buildFolderTree(c, userID, rootID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}
	c.JSON(http.StatusOK, tree)
}

func (h *folderHandler) buildFolderTree(c *gin.Context, userID, folderID string) (map[string]interface{}, error) {
	var name, createdAt string
	err := h.db.QueryRowContext(c.Request.Context(),
		`SELECT name, created_at FROM folders WHERE id=$1 AND user_id=$2 AND deleted_at IS NULL`,
		folderID, userID,
	).Scan(&name, &createdAt)
	if err != nil {
		return nil, err
	}

	rows, err := h.db.QueryContext(c.Request.Context(),
		`SELECT id FROM folders WHERE parent_id=$1 AND user_id=$2 AND deleted_at IS NULL`,
		folderID, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	children := []map[string]interface{}{}
	for rows.Next() {
		var childID string
		_ = rows.Scan(&childID)
		childTree, err := h.buildFolderTree(c, userID, childID)
		if err == nil {
			children = append(children, childTree)
		}
	}

	return map[string]interface{}{
		"id":        folderID,
		"name":      name,
		"createdAt": createdAt,
		"children":  children,
	}, nil
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

	if folderID == req.TargetParentID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot move folder inside itself"})
		return
	}

	_, err := h.db.ExecContext(c.Request.Context(),
		`UPDATE folders SET parent_id=$1, updated_at=now() WHERE id=$2 AND user_id=$3`,
		req.TargetParentID, folderID, userID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "folder moved"})
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
	_, err := h.db.ExecContext(c.Request.Context(),
		`UPDATE folders SET name=$1, updated_at=now() WHERE id=$2 AND user_id=$3`,
		req.Name, folderID, userID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "folder renamed"})
}

func (h *folderHandler) delete(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	folderID := c.Param("id")
	_, err := h.db.ExecContext(c.Request.Context(),
		`UPDATE folders SET deleted_at=now() WHERE id=$1 AND user_id=$2`,
		folderID, userID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "folder deleted"})
}
