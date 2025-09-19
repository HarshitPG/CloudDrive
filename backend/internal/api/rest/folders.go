package rest

import (
	"database/sql"
	"net/http"
	"strconv"

	"backend/internal/auth"
	"backend/internal/cache"
	"backend/internal/worker"

	"github.com/gin-gonic/gin"
)

func RegisterFolderRoutes(rg *gin.RouterGroup, db *sql.DB, jwtSecret string) {
	folders := rg.Group("/folders")
	folders.Use(auth.RequireAuth(jwtSecret))
	h := &folderHandler{db: db, producer: worker.NewProducer()}

	folders.GET("", h.list)
	folders.POST("", h.create)
	folders.GET("/:id/contents", h.listContents)
	folders.GET("/:id/files", h.listFilesInFolder)
	folders.GET("/:id/tree", h.getTree)
	folders.PATCH("/:id", h.rename)
	folders.DELETE("/:id", h.delete)
	folders.POST("/:id/move", h.move)
}

type folderHandler struct {
	db       *sql.DB
	cache    cache.Cache
	producer *worker.Producer
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
			SELECT id, name, created_at, updated_at
			FROM folders
			WHERE user_id=$1 AND parent_id IS NULL AND deleted_at IS NULL
			ORDER BY created_at DESC
			LIMIT $2 OFFSET $3
		`, userID, limit, offset)
	} else {
		rows, err = h.db.QueryContext(c.Request.Context(), `
			SELECT id, name, created_at, updated_at
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
		if err := rows.Scan(&id, &name, &createdAt, &updatedAt); err != nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"id":        id,
			"name":      name,
			"createdAt": createdAt,
			"updatedAt": updatedAt,
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
	cache.InvalidateFolder(c.Request.Context(), h.cache, req.ParentID)
	c.JSON(http.StatusCreated, gin.H{"message": "folder created"})
}

func (h *folderHandler) listContents(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	folderID := c.Param("id")

	// Try cache first
	cacheKey := cache.FolderContentsKey(folderID)
	var cachedResponse gin.H
	if h.cache != nil {
		if err := h.cache.Get(c.Request.Context(), cacheKey, &cachedResponse); err == nil {
			c.JSON(http.StatusOK, cachedResponse)
			return
		}
	}
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

	resp := gin.H{
		"folders": subfolders,
		"files":   files,
	}

	// Write-through cache
	if h.cache != nil {
		_ = h.cache.Set(c.Request.Context(), cacheKey, resp, cache.TTLFolderList)
	}
	c.JSON(http.StatusOK, resp)
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

func (h *folderHandler) listFilesInFolder(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	folderID := c.Param("id")

	var folderOwner string
	err := h.db.QueryRowContext(c.Request.Context(),
		"SELECT user_id FROM folders WHERE id=$1 AND deleted_at IS NULL",
		folderID,
	).Scan(&folderOwner)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "folder not found"})
		return
	}
	if folderOwner != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "not authorized"})
		return
	}

	limit := 20
	if v := c.Query("limit"); v != "" {
		if n, e := strconv.Atoi(v); e == nil {
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
		if n, e := strconv.Atoi(v); e == nil {
			offset = n
		}
	}
	if offset < 0 {
		offset = 0
	}
	if offset > 100000 {
		offset = 100000
	}

	rows, qerr := h.db.QueryContext(c.Request.Context(), `
		SELECT uf.id, uf.filename, uf.declared_mime, uf.original_size_bytes,
			   uf.created_at, uf.updated_at, uf.download_count,
			   fc.content_hash, fc.size_bytes, fc.ref_count
		FROM user_files uf
		JOIN file_contents fc ON uf.content_id = fc.id
		WHERE uf.user_id=$1 AND uf.folder_id=$2 AND uf.deleted_at IS NULL
		ORDER BY uf.created_at DESC
		LIMIT $3 OFFSET $4
	`, userID, folderID, limit, offset)
	if qerr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}
	defer rows.Close()

	files := make([]map[string]interface{}, 0, limit)
	for rows.Next() {
		var id, filename, mime, contentHash string
		var size, contentSize, refCount, downloadCount int64
		var createdAt, updatedAt string
		if err := rows.Scan(&id, &filename, &mime, &size,
			&createdAt, &updatedAt, &downloadCount,
			&contentHash, &contentSize, &refCount); err != nil {
			continue
		}
		files = append(files, map[string]interface{}{
			"id":            id,
			"filename":      filename,
			"mime":          mime,
			"size":          size,
			"createdAt":     createdAt,
			"updatedAt":     updatedAt,
			"downloadCount": downloadCount,
			"contentHash":   contentHash,
			"physicalSize":  contentSize,
			"refCount":      refCount,
			"dedupSavings":  size - contentSize,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"files": files,
		"page": gin.H{
			"limit":  limit,
			"offset": offset,
		},
	})
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

	// cache invalidation
	cache.InvalidateFolder(c.Request.Context(), h.cache, folderID)
	cache.InvalidateFolder(c.Request.Context(), h.cache, req.TargetParentID)

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
	// cache invalidation
	cache.InvalidateFolder(c.Request.Context(), h.cache, folderID)

	c.JSON(http.StatusOK, gin.H{"message": "folder renamed"})
}

func (h *folderHandler) delete(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	folderID := c.Param("id")

	permanent := c.Query("permanent") == "true"

	if !permanent {
		// Soft delete entire subtree: mark folders/files deleted, adjust ref_counts, revoke shares
		tx, _ := h.db.BeginTx(c.Request.Context(), nil)
		// Collect subtree folders
		rows, err := tx.QueryContext(c.Request.Context(), `
			WITH RECURSIVE subfolders AS (
				SELECT id FROM folders WHERE id=$1 AND user_id=$2
				UNION ALL
				SELECT f.id FROM folders f
				INNER JOIN subfolders sf ON f.parent_id = sf.id
			)
			SELECT id FROM subfolders
		`, folderID, userID)
		if err != nil {
			_ = tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
			return
		}
		defer rows.Close()
		folderIDs := []string{}
		for rows.Next() {
			var id string
			_ = rows.Scan(&id)
			folderIDs = append(folderIDs, id)
		}
		// For each folder, soft delete files and adjust ref_counts
		for _, fid := range folderIDs {
			frows, ferr := tx.QueryContext(c.Request.Context(), `
				SELECT uf.id, uf.content_id, fc.blob_key
				FROM user_files uf
				JOIN file_contents fc ON uf.content_id = fc.id
				WHERE uf.user_id=$1 AND uf.folder_id=$2 AND uf.deleted_at IS NULL
			`, userID, fid)
			if ferr != nil {
				_ = tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
				return
			}
			for frows.Next() {
				var fileId, contentID, blobKey string
				_ = frows.Scan(&fileId, &contentID, &blobKey)
				if _, err := tx.ExecContext(c.Request.Context(),
					"UPDATE user_files SET deleted_at = now() WHERE id=$1", fileId); err != nil {
					_ = tx.Rollback()
					c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
					return
				}
				if _, err := tx.ExecContext(c.Request.Context(),
					"UPDATE file_contents SET ref_count = GREATEST(ref_count - 1, 0) WHERE id=$1", contentID); err != nil {
					_ = tx.Rollback()
					c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
					return
				}
				var refCount int64
				_ = tx.QueryRowContext(c.Request.Context(),
					"SELECT ref_count FROM file_contents WHERE id=$1", contentID).Scan(&refCount)
				if refCount == 0 && h.producer != nil {
					_ = h.producer.PublishGCJob(c.Request.Context(), worker.GCJob{
						ContentID: contentID,
						BlobKey:   blobKey,
					})
				}

				_, _ = tx.ExecContext(c.Request.Context(),
					"UPDATE shares SET revoked=true WHERE target_type='file' AND target_id=$1", fileId)
			}
			_ = frows.Close()

			_, _ = tx.ExecContext(c.Request.Context(),
				"UPDATE shares SET revoked=true WHERE target_type='folder' AND target_id=$1", fid)
		}
		// Mark folders in subtree as deleted
		if _, err := tx.ExecContext(c.Request.Context(), `
			WITH RECURSIVE subfolders AS (
				SELECT id FROM folders WHERE id=$1 AND user_id=$2
				UNION ALL
				SELECT f.id FROM folders f
				INNER JOIN subfolders sf ON f.parent_id = sf.id
			)
			UPDATE folders SET deleted_at = now() WHERE id IN (SELECT id FROM subfolders)
		`, folderID, userID); err != nil {
			_ = tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
			return
		}
		if err := tx.Commit(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "commit failed"})
			return
		}

		// cache invalidation
		cache.InvalidateFolder(c.Request.Context(), h.cache, folderID)
		cache.InvalidateSearch(c.Request.Context(), h.cache, userID)

		c.JSON(http.StatusOK, gin.H{"message": "folder moved to trash (subtree) and shares revoked"})
		return
	}

	// Permanent delete: recursively delete all descendant files and folders.
	// This will also decrement ref_counts and schedule GC where needed.
	tx, _ := h.db.BeginTx(c.Request.Context(), nil)

	// Collect all descendant folder ids including the root
	rows, err := tx.QueryContext(c.Request.Context(), `
		WITH RECURSIVE subfolders AS (
			SELECT id FROM folders WHERE id=$1 AND user_id=$2
			UNION ALL
			SELECT f.id FROM folders f
			INNER JOIN subfolders sf ON f.parent_id = sf.id
		)
		SELECT id FROM subfolders
	`, folderID, userID)
	if err != nil {
		_ = tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}
	defer rows.Close()

	folderIDs := []string{}
	for rows.Next() {
		var id string
		_ = rows.Scan(&id)
		folderIDs = append(folderIDs, id)
	}

	// For each folder, permanently delete files and adjust ref_counts
	for _, fid := range folderIDs {
		// Fetch files in this folder (including trashed ones)
		frows, ferr := tx.QueryContext(c.Request.Context(), `
			SELECT uf.id, uf.content_id, fc.blob_key
			FROM user_files uf
			JOIN file_contents fc ON uf.content_id = fc.id
			WHERE uf.user_id=$1 AND uf.folder_id=$2
		`, userID, fid)
		if ferr != nil {
			_ = tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
			return
		}
		for frows.Next() {
			var fileId, contentID, blobKey string
			_ = frows.Scan(&fileId, &contentID, &blobKey)
			// Delete the file row
			if _, err := tx.ExecContext(c.Request.Context(),
				"DELETE FROM user_files WHERE id=$1", fileId); err != nil {
				_ = tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
				return
			}
			// Decrement ref_count and if 0 schedule GC
			if _, err := tx.ExecContext(c.Request.Context(),
				"UPDATE file_contents SET ref_count = GREATEST(ref_count - 1, 0) WHERE id=$1", contentID); err != nil {
				_ = tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
				return
			}
			var refCount int64
			_ = tx.QueryRowContext(c.Request.Context(),
				"SELECT ref_count FROM file_contents WHERE id=$1", contentID).Scan(&refCount)
			if refCount == 0 && h.producer != nil {
				_ = h.producer.PublishGCJob(c.Request.Context(), worker.GCJob{
					ContentID: contentID,
					BlobKey:   blobKey,
				})
			}
			// Revoke shares on file
			_, _ = tx.ExecContext(c.Request.Context(),
				"UPDATE shares SET revoked=true WHERE target_type='file' AND target_id=$1", fileId)
		}
		_ = frows.Close()
		// Revoke shares on folder
		_, _ = tx.ExecContext(c.Request.Context(),
			"UPDATE shares SET revoked=true WHERE target_type='folder' AND target_id=$1", fid)
	}

	// Delete folders themselves
	// Order children first by deleting using recursive CTE
	if _, err := tx.ExecContext(c.Request.Context(), `
		WITH RECURSIVE subfolders AS (
			SELECT id FROM folders WHERE id=$1 AND user_id=$2
			UNION ALL
			SELECT f.id FROM folders f
			INNER JOIN subfolders sf ON f.parent_id = sf.id
		)
		DELETE FROM folders WHERE id IN (SELECT id FROM subfolders)
	`, folderID, userID); err != nil {
		_ = tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "commit failed"})
		return
	}

	cache.InvalidateFolder(c.Request.Context(), h.cache, folderID)
	cache.InvalidateSearch(c.Request.Context(), h.cache, userID)
	c.JSON(http.StatusOK, gin.H{"message": "folder permanently deleted"})
}
