package rest

import (
	"archive/zip"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"

	"backend/internal/auth"
	"backend/internal/cache"
	"backend/internal/storage"
	"backend/internal/worker"
	"backend/pkg/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func RegisterFolderRoutes(rg *gin.RouterGroup, db *sql.DB, jwtSecret string, c cache.Cache, st *storage.MinioStorage) {
	folders := rg.Group("/folders")
	folders.Use(auth.RequireAuth(jwtSecret))
	h := &folderHandler{db: db, cache: c, producer: worker.NewProducer(), storage: st}

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

// listPrimary returns only primary folders: folders whose parent is NULL or whose parent folder is not in trash.
// It supports pagination and the 'deleted' flag to list trashed folders that are primary.
func (h *folderHandler) listPrimary(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	parentID := c.Query("parentId")
	deleted := c.Query("deleted") == "true"

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

	var rows *sql.Rows
	var err error

	if deleted {
		// list trashed folders but only those whose parent is NULL or parent is not trashed
		rows, err = h.db.QueryContext(c.Request.Context(), `
					SELECT f.id, f.name, f.created_at, f.updated_at, f.deleted_at,
						COALESCE((SELECT SUM(fc.size_bytes)
											FROM user_files uf
											JOIN file_contents fc ON uf.content_id = fc.id
											WHERE uf.folder_id = f.id AND uf.deleted_at IS NULL), 0) AS size
			FROM folders f
			LEFT JOIN folders p ON f.parent_id = p.id
			WHERE f.user_id = $1 AND f.deleted_at IS NOT NULL
			  AND (f.parent_id IS NULL OR p.deleted_at IS NULL)
			ORDER BY f.deleted_at DESC
			LIMIT $2 OFFSET $3
		`, userID, limit, offset)
	} else if parentID == "" {
		// root primary folders
		rows, err = h.db.QueryContext(c.Request.Context(), `
			SELECT f.id, f.name, f.created_at, f.updated_at,
				COALESCE((SELECT SUM(fc.size_bytes)
						  FROM user_files uf
						  JOIN file_contents fc ON uf.content_id = fc.id
						  WHERE uf.folder_id = f.id AND uf.deleted_at IS NULL), 0) AS size
			FROM folders f
			WHERE f.user_id = $1 AND f.parent_id IS NULL AND f.deleted_at IS NULL
			ORDER BY f.created_at DESC
			LIMIT $2 OFFSET $3
		`, userID, limit, offset)
	} else {
		// list children of a parent only if parent is not trashed
		rows, err = h.db.QueryContext(c.Request.Context(), `
			SELECT f.id, f.name, f.created_at, f.updated_at,
				COALESCE((SELECT SUM(fc.size_bytes)
						  FROM user_files uf
						  JOIN file_contents fc ON uf.content_id = fc.id
						  WHERE uf.folder_id = f.id AND uf.deleted_at IS NULL), 0) AS size
			FROM folders f
			JOIN folders p ON f.parent_id = p.id
			WHERE f.user_id = $1 AND f.parent_id = $2 AND f.deleted_at IS NULL AND p.deleted_at IS NULL
			ORDER BY f.created_at DESC
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
		var deletedAt sql.NullTime
		if deleted {
			var size sql.NullInt64
			if err := rows.Scan(&id, &name, &createdAt, &updatedAt, &deletedAt, &size); err != nil {
				continue
			}
			s := int64(0)
			if size.Valid {
				s = size.Int64
			}
			item := map[string]interface{}{
				"id":        id,
				"name":      name,
				"createdAt": createdAt,
				"updatedAt": updatedAt,
				"size":      s,
			}
			if deletedAt.Valid {
				item["deletedAt"] = deletedAt.Time
			}
			out = append(out, item)
			continue
		} else {
			var size sql.NullInt64
			if err := rows.Scan(&id, &name, &createdAt, &updatedAt, &size); err != nil {
				continue
			}
			s := int64(0)
			if size.Valid {
				s = size.Int64
			}
			item := map[string]interface{}{
				"id":        id,
				"name":      name,
				"createdAt": createdAt,
				"updatedAt": updatedAt,
				"size":      s,
			}
			out = append(out, item)
			continue
		}

	}

	c.JSON(http.StatusOK, gin.H{
		"folders": out,
		"page":    gin.H{"limit": limit, "offset": offset},
	})
}

func (h *folderHandler) create(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	var req createFolderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var newID string
	err := h.db.QueryRowContext(c.Request.Context(), `
		INSERT INTO folders (id, user_id, parent_id, name, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, NULLIF($2,'')::uuid, $3, now(), now())
		RETURNING id
	`, userID, req.ParentID, req.Name).Scan(&newID)
	if err != nil {
		logger.L.Error("folders.create: insert failed", zap.Error(err), zap.String("userID", userID), zap.String("parentId", req.ParentID), zap.String("name", req.Name))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}

	if req.ParentID != "" {
		cache.InvalidateFolder(c.Request.Context(), h.cache, req.ParentID)
	}

	c.JSON(http.StatusCreated, gin.H{"id": newID, "message": "folder created"})
}

// listContents returns immediate child folders and files for a folder
func (h *folderHandler) listContents(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	folderID := c.Param("id")

	var owner string
	if err := h.db.QueryRowContext(c.Request.Context(), `SELECT user_id FROM folders WHERE id=$1 AND deleted_at IS NULL`, folderID).Scan(&owner); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "folder not found"})
		return
	}
	if owner != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "not authorized"})
		return
	}

	cacheKey := cache.FolderContentsKey(folderID)
	if h.cache != nil {
		var cached gin.H
		if err := h.cache.Get(c.Request.Context(), cacheKey, &cached); err == nil {
			c.JSON(http.StatusOK, cached)
			return
		}
	}

	sfRows, err := h.db.QueryContext(c.Request.Context(), `
		SELECT id, name, created_at FROM folders
		WHERE user_id=$1 AND parent_id=$2 AND deleted_at IS NULL
		ORDER BY created_at DESC
	`, userID, folderID)
	if err != nil {
		logger.L.Error("folders.listContents: subfolders query failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}
	defer sfRows.Close()
	subs := []map[string]interface{}{}
	for sfRows.Next() {
		var id, name, createdAt string
		if err := sfRows.Scan(&id, &name, &createdAt); err != nil {
			continue
		}
		subs = append(subs, map[string]interface{}{"id": id, "name": name, "createdAt": createdAt})
	}

	fRows, err := h.db.QueryContext(c.Request.Context(), `
		SELECT id, filename, declared_mime, original_size_bytes, created_at
		FROM user_files
		WHERE user_id=$1 AND folder_id=$2 AND deleted_at IS NULL
		ORDER BY created_at DESC
	`, userID, folderID)
	if err != nil {
		logger.L.Error("folders.listContents: files query failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}
	defer fRows.Close()
	files := []map[string]interface{}{}
	for fRows.Next() {
		var id, filename, mime, createdAt string
		var size int64
		if err := fRows.Scan(&id, &filename, &mime, &size, &createdAt); err != nil {
			continue
		}
		files = append(files, map[string]interface{}{"id": id, "filename": filename, "mime": mime, "size": size, "createdAt": createdAt})
	}

	resp := gin.H{"folders": subs, "files": files}
	if h.cache != nil {
		_ = h.cache.Set(c.Request.Context(), cacheKey, resp, cache.TTLFolderList)
	}
	c.JSON(http.StatusOK, resp)
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
	cache.InvalidateFolder(c.Request.Context(), h.cache, folderID)
	c.JSON(http.StatusOK, gin.H{"message": "folder renamed"})
}

// downloadArchive streams the folder contents as a ZIP archive to the authenticated owner
func (h *folderHandler) downloadArchive(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	folderID := c.Param("id")

	var owner string
	if err := h.db.QueryRowContext(c.Request.Context(), `SELECT user_id FROM folders WHERE id=$1 AND deleted_at IS NULL`, folderID).Scan(&owner); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "folder not found"})
		return
	}
	if owner != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "not authorized"})
		return
	}

	recursive := c.Query("recursive") == "true"

	// Get files
	var items []shareItem
	if recursive {
		items, _ = getRecursiveItemsStandalone(c.Request.Context(), h.db, folderID, owner)
	} else {
		items, _ = getDirectItemsStandalone(c.Request.Context(), h.db, folderID, owner)
	}

	var files []shareItem
	for _, it := range items {
		if it.Type == "file" {
			files = append(files, it)
		}
	}

	if len(files) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "no files found in folder"})
		return
	}

	// Get folder name for zip filename
	var folderName string
	_ = h.db.QueryRowContext(c.Request.Context(), `SELECT name FROM folders WHERE id=$1`, folderID).Scan(&folderName)
	if folderName == "" {
		folderName = "folder"
	}

	zipFilename := folderName + ".zip"
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", zipFilename))
	c.Header("Cache-Control", "no-cache")

	zipWriter := zip.NewWriter(c.Writer)
	defer zipWriter.Close()

	for _, file := range files {
		if err := addFileToZip(c.Request.Context(), h.db, h.storage, zipWriter, file); err != nil {
			logger.L.Warn("failed to add file to zip", zap.String("fileId", file.ID), zap.Error(err))
			continue
		}
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
	cache.InvalidateFolder(c.Request.Context(), h.cache, folderID)
	if req.TargetParentID != "" {
		cache.InvalidateFolder(c.Request.Context(), h.cache, req.TargetParentID)
	}
	c.JSON(http.StatusOK, gin.H{"message": "folder moved"})
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

// getAncestors returns the ancestor chain for a folder owned by the authenticated user.
// Response: [{id,name}, ...] ordered root -> current
func (h *folderHandler) getAncestors(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	folderID := c.Param("id")

	rows, err := h.db.QueryContext(c.Request.Context(), `
		WITH RECURSIVE anc AS (
			SELECT id, parent_id, name FROM folders WHERE id=$1 AND user_id=$2
			UNION ALL
			SELECT f.id, f.parent_id, f.name FROM folders f
			JOIN anc a ON f.id = a.parent_id
			WHERE f.user_id = $2
		)
		SELECT id, name FROM anc
	`, folderID, userID)
	if err != nil {
		logger.L.Error("folders.getAncestors: query failed", zap.Error(err), zap.String("folderID", folderID), zap.String("userID", userID))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}
	defer rows.Close()

	type ancRow struct{ id, name string }
	list := make([]ancRow, 0)
	for rows.Next() {
		var a ancRow
		if err := rows.Scan(&a.id, &a.name); err != nil {
			continue
		}
		list = append(list, a)
	}

	// rows come current->parent->...; reverse to root->current
	for i, j := 0, len(list)-1; i < j; i, j = i+1, j-1 {
		list[i], list[j] = list[j], list[i]
	}

	out := make([]map[string]string, 0, len(list))
	for _, a := range list {
		out = append(out, map[string]string{"id": a.id, "name": a.name})
	}
	c.JSON(http.StatusOK, gin.H{"ancestors": out})
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

func (h *folderHandler) delete(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	folderID := c.Param("id")

	permanent := c.Query("permanent") == "true"

	if !permanent {
		// Soft delete entire subtree: mark folders/files deleted, adjust ref_counts, revoke shares
		tx, err := h.db.BeginTx(c.Request.Context(), nil)
		if err != nil {
			logger.L.Error("folders.softdelete: begin tx failed", zap.Error(err), zap.String("userID", userID), zap.String("folderID", folderID))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
			return
		}

		// Collect all files within the subtree first to avoid executing statements while a cursor is open
		type fileRow struct {
			fileID    string
			contentID string
			blobKey   string
		}
		files := make([]fileRow, 0, 64)
		frows, err := tx.QueryContext(c.Request.Context(), `
			WITH RECURSIVE subfolders AS (
				SELECT id FROM folders WHERE id=$1 AND user_id=$2
				UNION ALL
				SELECT f.id FROM folders f
				INNER JOIN subfolders sf ON f.parent_id = sf.id
				WHERE f.user_id = $2
			)
			SELECT uf.id, uf.content_id, fc.blob_key
			FROM user_files uf
			JOIN file_contents fc ON uf.content_id = fc.id
			WHERE uf.user_id=$2 AND uf.folder_id IN (SELECT id FROM subfolders) AND uf.deleted_at IS NULL
		`, folderID, userID)
		if err != nil {
			_ = tx.Rollback()
			logger.L.Error("folders.softdelete: list subtree files failed", zap.Error(err), zap.String("userID", userID), zap.String("folderID", folderID))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
			return
		}
		for frows.Next() {
			var fr fileRow
			if scanErr := frows.Scan(&fr.fileID, &fr.contentID, &fr.blobKey); scanErr != nil {
				_ = frows.Close()
				_ = tx.Rollback()
				logger.L.Error("folders.softdelete: scan subtree file row failed", zap.Error(scanErr))
				c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
				return
			}
			files = append(files, fr)
		}
		_ = frows.Close()

		// Now perform updates for files
		for _, fr := range files {
			if _, err := tx.ExecContext(c.Request.Context(),
				"UPDATE user_files SET deleted_at = now() WHERE id=$1", fr.fileID); err != nil {
				_ = tx.Rollback()
				logger.L.Error("folders.softdelete: mark file deleted failed", zap.Error(err), zap.String("fileID", fr.fileID))
				c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
				return
			}
			if _, err := tx.ExecContext(c.Request.Context(),
				"UPDATE file_contents SET ref_count = GREATEST(ref_count - 1, 0) WHERE id=$1", fr.contentID); err != nil {
				_ = tx.Rollback()
				logger.L.Error("folders.softdelete: decrement ref_count failed", zap.Error(err), zap.String("contentID", fr.contentID))
				c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
				return
			}
			if _, err := tx.ExecContext(c.Request.Context(),
				"UPDATE shares SET revoked=true WHERE target_type='file' AND target_id=$1", fr.fileID); err != nil {
				logger.L.Warn("folders.softdelete: revoke file shares failed", zap.Error(err), zap.String("fileID", fr.fileID))
			}
		}

		// Revoke shares for all folders in subtree at once
		if _, err := tx.ExecContext(c.Request.Context(), `
			WITH RECURSIVE subfolders AS (
				SELECT id FROM folders WHERE id=$1 AND user_id=$2
				UNION ALL
				SELECT f.id FROM folders f
				INNER JOIN subfolders sf ON f.parent_id = sf.id
				WHERE f.user_id = $2
			)
			UPDATE shares SET revoked=true WHERE target_type='folder' AND target_id IN (SELECT id FROM subfolders)
		`, folderID, userID); err != nil {
			logger.L.Warn("folders.softdelete: revoke folder shares failed", zap.Error(err), zap.String("folderID", folderID))
		}

		// Mark folders in subtree as deleted
		if _, err := tx.ExecContext(c.Request.Context(), `
			WITH RECURSIVE subfolders AS (
				SELECT id FROM folders WHERE id=$1 AND user_id=$2
				UNION ALL
				SELECT f.id FROM folders f
				INNER JOIN subfolders sf ON f.parent_id = sf.id
				WHERE f.user_id = $2
			)
			UPDATE folders SET deleted_at = now() WHERE id IN (SELECT id FROM subfolders)
		`, folderID, userID); err != nil {
			_ = tx.Rollback()
			logger.L.Error("folders.softdelete: mark folders deleted failed", zap.Error(err), zap.String("userID", userID), zap.String("folderID", folderID))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
			return
		}
		if err := tx.Commit(); err != nil {
			logger.L.Error("folders.softdelete: commit failed", zap.Error(err))
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
	tx, err := h.db.BeginTx(c.Request.Context(), nil)
	if err != nil {
		logger.L.Error("folders.harddelete: begin tx failed", zap.Error(err), zap.String("userID", userID), zap.String("folderID", folderID))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	// Collect all files within the subtree first (including trashed ones)
	type fileRow struct {
		fileID    string
		contentID string
		blobKey   string
	}
	files := make([]fileRow, 0, 64)
	frows, err := tx.QueryContext(c.Request.Context(), `
		WITH RECURSIVE subfolders AS (
			SELECT id FROM folders WHERE id=$1 AND user_id=$2
			UNION ALL
			SELECT f.id FROM folders f
			INNER JOIN subfolders sf ON f.parent_id = sf.id
			WHERE f.user_id = $2
		)
		SELECT uf.id, uf.content_id, fc.blob_key
		FROM user_files uf
		JOIN file_contents fc ON uf.content_id = fc.id
		WHERE uf.user_id=$2 AND uf.folder_id IN (SELECT id FROM subfolders)
	`, folderID, userID)
	if err != nil {
		_ = tx.Rollback()
		logger.L.Error("folders.harddelete: list subtree files failed", zap.Error(err), zap.String("userID", userID), zap.String("folderID", folderID))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}
	for frows.Next() {
		var fr fileRow
		if scanErr := frows.Scan(&fr.fileID, &fr.contentID, &fr.blobKey); scanErr != nil {
			_ = frows.Close()
			_ = tx.Rollback()
			logger.L.Error("folders.harddelete: scan file row failed", zap.Error(scanErr))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
			return
		}
		files = append(files, fr)
	}
	_ = frows.Close()

	// Now perform deletes/updates for each file without an open cursor
	for _, fr := range files {
		// Delete the user_files row first so the FK from user_files -> file_contents is removed
		if _, err := tx.ExecContext(c.Request.Context(), "DELETE FROM user_files WHERE id=$1", fr.fileID); err != nil {
			_ = tx.Rollback()
			logger.L.Error("folders.harddelete: delete user_file failed", zap.Error(err), zap.String("fileID", fr.fileID))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
			return
		}
		// Decrement ref_count and get the resulting value
		var refCount int64
		if err := tx.QueryRowContext(c.Request.Context(),
			"UPDATE file_contents SET ref_count = GREATEST(ref_count - 1, 0) WHERE id=$1 RETURNING ref_count",
			fr.contentID,
		).Scan(&refCount); err != nil {
			_ = tx.Rollback()
			logger.L.Error("folders.harddelete: refcount decrement failed", zap.Error(err), zap.String("contentID", fr.contentID))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
			return
		}
		// Only publish GC and delete the file_contents row if ref_count is zero AND no remaining user_files reference it.
		if refCount == 0 {
			// As a safety check, ensure there are no lingering user_files referencing this content.
			var remaining int64
			if err := tx.QueryRowContext(c.Request.Context(), "SELECT COUNT(1) FROM user_files WHERE content_id=$1", fr.contentID).Scan(&remaining); err != nil {
				_ = tx.Rollback()
				logger.L.Error("folders.harddelete: check remaining user_files failed", zap.Error(err), zap.String("contentID", fr.contentID))
				c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
				return
			}
			if remaining == 0 {
				if h.producer != nil {
					if err := h.producer.PublishGCJob(c.Request.Context(), worker.GCJob{
						ContentID: fr.contentID,
						BlobKey:   fr.blobKey,
					}); err != nil {
						logger.L.Warn("folders.harddelete: publish GC job failed", zap.Error(err), zap.String("contentID", fr.contentID))
					}
				}
				if _, err := tx.ExecContext(c.Request.Context(), "DELETE FROM file_contents WHERE id=$1", fr.contentID); err != nil {
					_ = tx.Rollback()
					logger.L.Error("folders.harddelete: delete file_contents failed", zap.Error(err), zap.String("contentID", fr.contentID))
					c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
					return
				}
			}
		}
		if _, err := tx.ExecContext(c.Request.Context(),
			"DELETE FROM shares WHERE target_type='file' AND target_id=$1", fr.fileID); err != nil {
			logger.L.Warn("folders.harddelete: delete file shares failed", zap.Error(err), zap.String("fileID", fr.fileID))
		}
	}

	// Remove shares on folders in subtree (bulk)
	if _, err := tx.ExecContext(c.Request.Context(), `
		WITH RECURSIVE subfolders AS (
			SELECT id FROM folders WHERE id=$1 AND user_id=$2
			UNION ALL
			SELECT f.id FROM folders f
			INNER JOIN subfolders sf ON f.parent_id = sf.id
			WHERE f.user_id = $2
		)
		DELETE FROM shares WHERE target_type='folder' AND target_id IN (SELECT id FROM subfolders)
	`, folderID, userID); err != nil {
		logger.L.Warn("folders.harddelete: delete folder shares failed", zap.Error(err), zap.String("folderID", folderID))
	}

	// Delete folders themselves
	if _, err := tx.ExecContext(c.Request.Context(), `
		WITH RECURSIVE subfolders AS (
			SELECT id FROM folders WHERE id=$1 AND user_id=$2
			UNION ALL
			SELECT f.id FROM folders f
			INNER JOIN subfolders sf ON f.parent_id = sf.id
			WHERE f.user_id = $2
		)
		DELETE FROM folders WHERE id IN (SELECT id FROM subfolders)
	`, folderID, userID); err != nil {
		_ = tx.Rollback()
		logger.L.Error("folders.harddelete: delete folders failed", zap.Error(err), zap.String("userID", userID), zap.String("folderID", folderID))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}

	if err := tx.Commit(); err != nil {
		logger.L.Error("folders.harddelete: commit failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "commit failed"})
		return
	}

	cache.InvalidateFolder(c.Request.Context(), h.cache, folderID)
	cache.InvalidateSearch(c.Request.Context(), h.cache, userID)
	c.JSON(http.StatusOK, gin.H{"message": "folder permanently deleted"})
}
