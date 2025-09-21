package rest

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/http"
	"time"

	"backend/internal/auth"
	"backend/internal/cache"
	"backend/internal/storage"
	"backend/pkg/logger"

	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func RegisterShareRoutes(rg *gin.RouterGroup, db *sql.DB, st *storage.MinioStorage, jwtSecret string, c cache.Cache) {
	rg.GET("/s/:token", resolveShareHandler(db, st, c))

	protected := rg.Group("/shares")
	protected.Use(auth.RequireAuth(jwtSecret))
	h := &shareHandler{db: db, storage: st, cache: c}

	protected.POST("/files/:id/share", h.createPublicFileShare)
	protected.GET("/files/:id/download", h.downloadSharedFile)
	protected.DELETE("/shares/:id", h.revokeShare)
	protected.POST("/files/:id/share/user", h.shareFileToUser)
	protected.GET("/files/:id/shares", h.listFileShares)
	protected.POST("/folders/:id/share", h.createPublicFolderShare)
	protected.POST("/folders/:id/share/user", h.shareFolderToUser)
	protected.GET("/folders/:id/contents", h.listSharedFolderContents)
	protected.GET("/folders/:id/ancestors", h.listSharedFolderAncestors)
	protected.GET("/shared-with-me", h.listSharedWithMe)
}

type shareHandler struct {
	db      *sql.DB
	storage *storage.MinioStorage
	cache   cache.Cache
}

func genToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

type createShareReq struct {
	Title       string     `json:"title"`
	Description string     `json:"description"`
	ExpiresAt   *time.Time `json:"expiresAt"`
}

func (h *shareHandler) createPublicFileShare(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	fileId := c.Param("id")
	var owner string
	err := h.db.QueryRowContext(c.Request.Context(),
		"SELECT user_id FROM user_files WHERE id=$1 AND deleted_at IS NULL", fileId).Scan(&owner)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
			return
		}
		logger.L.Error("db err", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	if owner != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "not owner"})
		return
	}

	var req createShareReq
	if err := c.ShouldBindJSON(&req); err != nil {
		if err.Error() != "EOF" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON"})
			return
		}
	}

	token, err := genToken()
	if err != nil {
		logger.L.Error("token gen failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}

	var shareID string
	err = h.db.QueryRowContext(c.Request.Context(), `
		INSERT INTO shares (id, token, creator_id, target_type, target_id, title, description, expires_at, created_at)
		VALUES (gen_random_uuid(), $1, $2, 'file', $3, $4, $5, $6, now())
		RETURNING id
	`, token, userID, fileId, req.Title, req.Description, req.ExpiresAt).Scan(&shareID)
	if err != nil {
		logger.L.Error("insert share failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}

	_, _ = h.db.ExecContext(c.Request.Context(), "UPDATE user_files SET is_public = true WHERE id=$1", fileId)

	// Invalidate search
	if h.cache != nil {
		cache.InvalidateSearch(c.Request.Context(), h.cache, userID)
		cache.InvalidateShare(c.Request.Context(), h.cache, shareID)
	}

	c.JSON(http.StatusCreated, gin.H{
		"shareId": shareID,
		"token":   token,
		"url":     "/s/" + token,
	})
}

func (h *shareHandler) createPublicFolderShare(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	folderId := c.Param("id")
	var owner string
	err := h.db.QueryRowContext(c.Request.Context(),
		"SELECT user_id FROM folders WHERE id=$1 AND deleted_at IS NULL", folderId).Scan(&owner)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "folder not found"})
			return
		}
		logger.L.Error("db err", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	if owner != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "not owner"})
		return
	}

	var req createShareReq
	_ = c.ShouldBindJSON(&req)

	token, err := genToken()
	if err != nil {
		logger.L.Error("token gen failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}

	var shareID string
	err = h.db.QueryRowContext(c.Request.Context(), `
		INSERT INTO shares (id, token, creator_id, target_type, target_id, title, description, expires_at, created_at)
		VALUES (gen_random_uuid(), $1, $2, 'folder', $3, $4, $5, $6, now())
		RETURNING id
	`, token, userID, folderId, req.Title, req.Description, req.ExpiresAt).Scan(&shareID)
	if err != nil {
		logger.L.Error("insert share failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"shareId": shareID,
		"token":   token,
		"url":     "/s/" + token,
	})
}

type shareToUserReq struct {
	TargetUserEmail string `json:"targetUserEmail" binding:"required,email"`
	Permission      string `json:"permission"`
}

func (h *shareHandler) shareFileToUser(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	fileId := c.Param("id")

	var owner string
	err := h.db.QueryRowContext(c.Request.Context(),
		"SELECT user_id FROM user_files WHERE id=$1 AND deleted_at IS NULL", fileId).Scan(&owner)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
			return
		}
		logger.L.Error("db err", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	if owner != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "not owner"})
		return
	}

	var req shareToUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var targetUserID string
	err = h.db.QueryRowContext(c.Request.Context(), "SELECT id FROM users WHERE email=$1", req.TargetUserEmail).Scan(&targetUserID)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "target user not found"})
			return
		}
		logger.L.Error("db err", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}

	token, _ := genToken()
	var shareID string
	err = h.db.QueryRowContext(c.Request.Context(), `
		INSERT INTO shares (id, token, creator_id, target_type, target_id, title, description, expires_at, created_at)
		VALUES (gen_random_uuid(), $1, $2, 'file', $3, NULL, NULL, NULL, now())
		RETURNING id
	`, token, userID, fileId).Scan(&shareID)
	if err != nil {
		logger.L.Error("insert share record failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}

	_, err = h.db.ExecContext(c.Request.Context(), `
		INSERT INTO share_users (share_id, target_user_id, permission, created_at)
		VALUES ($1, $2, $3, now())
	`, shareID, targetUserID, req.Permission)
	if err != nil {
		logger.L.Error("insert share_user failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"shareId": shareID})
}

func (h *shareHandler) shareFolderToUser(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	folderID := c.Param("id")

	var owner string
	err := h.db.QueryRowContext(c.Request.Context(),
		"SELECT user_id FROM folders WHERE id=$1 AND deleted_at IS NULL", folderID,
	).Scan(&owner)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "folder not found"})
			return
		}
		logger.L.Error("db err", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	if owner != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "not owner"})
		return
	}

	var req shareToUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// Enforce sensible default permission
	permission := req.Permission
	if permission == "" {
		permission = "read"
	}

	var targetUserID string
	err = h.db.QueryRowContext(c.Request.Context(), "SELECT id FROM users WHERE email=$1", req.TargetUserEmail).Scan(&targetUserID)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "target user not found"})
			return
		}
		logger.L.Error("db err", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}

	// Create share and share_users in a transaction for atomicity
	tx, err := h.db.BeginTx(c.Request.Context(), nil)
	if err != nil {
		logger.L.Error("begin tx failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	defer func() {
		// Ensure rollback on early returns
		_ = tx.Rollback()
	}()

	token, terr := genToken()
	if terr != nil {
		logger.L.Error("token gen failed", zap.Error(terr))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}

	var shareID string
	err = tx.QueryRowContext(c.Request.Context(), `
		INSERT INTO shares (id, token, creator_id, target_type, target_id, title, description, expires_at, created_at)
		VALUES (gen_random_uuid(), $1, $2, 'folder', $3, NULL, NULL, NULL, now())
		RETURNING id
	`, token, userID, folderID).Scan(&shareID)
	if err != nil {
		logger.L.Error("insert share record failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}

	_, err = tx.ExecContext(c.Request.Context(), `
		INSERT INTO share_users (share_id, target_user_id, permission, created_at)
		VALUES ($1, $2, $3, now())
	`, shareID, targetUserID, permission)
	if err != nil {
		logger.L.Error("insert share_user failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}

	if err = tx.Commit(); err != nil {
		logger.L.Error("commit share tx failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}

	// No specific caches to invalidate for private folder shares currently.
	// If you add folder-sharing listings with caching, invalidate here.

	c.JSON(http.StatusCreated, gin.H{"shareId": shareID})
}

func (h *shareHandler) listFileShares(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	fileId := c.Param("id")

	var owner string
	err := h.db.QueryRowContext(c.Request.Context(),
		"SELECT user_id FROM user_files WHERE id=$1", fileId).Scan(&owner)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	if owner != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "not owner"})
		return
	}

	var public struct {
		ID        string
		Token     string
		Title     sql.NullString
		Desc      sql.NullString
		ExpiresAt sql.NullTime
		CreatedAt time.Time
	}
	row := h.db.QueryRowContext(c.Request.Context(), `
		SELECT id, token, title, description, expires_at, created_at
		FROM shares
		WHERE target_type='file' AND target_id=$1 AND revoked=false
		  AND (expires_at IS NULL OR expires_at > now())
		ORDER BY created_at DESC LIMIT 1
	`, fileId)

	_ = row.Scan(&public.ID, &public.Token, &public.Title, &public.Desc, &public.ExpiresAt, &public.CreatedAt)

	rows, err := h.db.QueryContext(c.Request.Context(), `
		SELECT su.id, su.target_user_id, su.permission, su.created_at
		FROM share_users su
		JOIN shares s ON su.share_id = s.id
		WHERE s.target_type='file' AND s.target_id=$1
	`, fileId)
	if err != nil {
		logger.L.Error("list share_users failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	defer rows.Close()

	userShares := []map[string]interface{}{}
	for rows.Next() {
		var id, uid, perm string
		var createdAt time.Time
		_ = rows.Scan(&id, &uid, &perm, &createdAt)
		userShares = append(userShares, map[string]interface{}{
			"shareUserId": id,
			"userId":      uid,
			"permission":  perm,
			"createdAt":   createdAt,
		})
	}

	resp := gin.H{"userShares": userShares}
	if public.ID != "" {
		resp["publicShare"] = gin.H{
			"id":          public.ID,
			"token":       public.Token,
			"title":       public.Title.String,
			"description": public.Desc.String,
			"expiresAt":   public.ExpiresAt.Time,
			"createdAt":   public.CreatedAt,
			"url":         "/s/" + public.Token,
		}
	}
	c.JSON(http.StatusOK, resp)
}

func (h *shareHandler) revokeShare(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	shareId := c.Param("id")

	var creator string
	err := h.db.QueryRowContext(c.Request.Context(),
		"SELECT creator_id, target_type, target_id FROM shares WHERE id=$1", shareId).
		Scan(&creator, new(string), new(string))
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "share not found"})
			return
		}
		logger.L.Error("db err", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	if creator != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "not creator"})
		return
	}

	_, err = h.db.ExecContext(c.Request.Context(), "UPDATE shares SET revoked=true WHERE id=$1", shareId)
	if err != nil {
		logger.L.Error("revoke share failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}

	// Invalidate cache
	if h.cache != nil {
		cache.InvalidateShare(c.Request.Context(), h.cache, shareId)
	}

	c.JSON(http.StatusOK, gin.H{"message": "revoked"})
}

// listSharedFolderContents lists immediate child folders and files of a folder that the current user can access via a folder share (on this folder or any ancestor).
func (h *shareHandler) listSharedFolderContents(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	folderID := c.Param("id")

	// Authorization: user must have a folder share on this folder or an ancestor
	var authorized int
	authErr := h.db.QueryRowContext(c.Request.Context(), `
		WITH RECURSIVE ancestors AS (
			SELECT id, parent_id FROM folders WHERE id = $1 AND deleted_at IS NULL
			UNION ALL
			SELECT f.id, f.parent_id FROM folders f
			JOIN ancestors a ON f.id = a.parent_id
		)
		SELECT 1
		FROM shares s
		JOIN share_users su ON su.share_id = s.id
		WHERE su.target_user_id = $2
		  AND s.target_type = 'folder'
		  AND s.revoked = false
		  AND s.target_id IN (SELECT id FROM ancestors)
		LIMIT 1
	`, folderID, userID).Scan(&authorized)
	if authErr != nil {
		if authErr == sql.ErrNoRows {
			c.JSON(http.StatusForbidden, gin.H{"error": "not authorized"})
			return
		}
		logger.L.Error("shares.listSharedFolderContents: auth query failed", zap.Error(authErr))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}

	// Fetch current folder name and parent for breadcrumb
	var curName string
	var parentID sql.NullString
	if err := h.db.QueryRowContext(c.Request.Context(),
		"SELECT name, parent_id::text FROM folders WHERE id=$1 AND deleted_at IS NULL",
		folderID,
	).Scan(&curName, &parentID); err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "folder not found"})
			return
		}
		logger.L.Error("shares.listSharedFolderContents: fetch current folder failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}

	// List child folders
	fRows, err := h.db.QueryContext(c.Request.Context(), `
		SELECT id, name, created_at
		FROM folders
		WHERE parent_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
	`, folderID)
	if err != nil {
		logger.L.Error("shares.listSharedFolderContents: subfolders query failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	defer fRows.Close()
	subfolders := make([]map[string]interface{}, 0)
	for fRows.Next() {
		var id, name string
		var createdAt interface{}
		if err := fRows.Scan(&id, &name, &createdAt); err == nil {
			subfolders = append(subfolders, gin.H{
				"id":        id,
				"name":      name,
				"createdAt": createdAt,
			})
		}
	}

	// List files in this folder
	fileRows, err := h.db.QueryContext(c.Request.Context(), `
		SELECT uf.id, uf.filename, uf.declared_mime, uf.original_size_bytes, uf.created_at
		FROM user_files uf
		WHERE uf.folder_id = $1 AND uf.deleted_at IS NULL
		ORDER BY uf.created_at DESC
	`, folderID)
	if err != nil {
		logger.L.Error("shares.listSharedFolderContents: files query failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	defer fileRows.Close()
	files := make([]map[string]interface{}, 0)
	for fileRows.Next() {
		var id, filename, mime string
		var size int64
		var createdAt interface{}
		if err := fileRows.Scan(&id, &filename, &mime, &size, &createdAt); err == nil {
			files = append(files, gin.H{
				"id":        id,
				"filename":  filename,
				"mime":      mime,
				"size":      size,
				"createdAt": createdAt,
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"folders": subfolders,
		"files":   files,
		"current": gin.H{
			"id":   folderID,
			"name": curName,
			"parentId": func() interface{} {
				if parentID.Valid {
					return parentID.String
				}
				return nil
			}(),
		},
	})
}

// downloadSharedFile returns a presigned download URL for a file that the current user can access via a direct file share or via a folder share ancestor.
func (h *shareHandler) downloadSharedFile(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	fileID := c.Param("id")

	// Fetch file content and its folder
	var (
		folderID string
		blobKey  string
		filename string
	)
	err := h.db.QueryRowContext(c.Request.Context(), `
		SELECT COALESCE(uf.folder_id::text, ''), fc.blob_key, uf.filename
		FROM user_files uf
		JOIN file_contents fc ON uf.content_id = fc.id
		WHERE uf.id = $1 AND uf.deleted_at IS NULL
	`, fileID).Scan(&folderID, &blobKey, &filename)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
			return
		}
		logger.L.Error("shares.downloadSharedFile: fetch file failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}

	// Check direct file share
	var hasDirect int
	derr := h.db.QueryRowContext(c.Request.Context(), `
		SELECT 1 FROM shares s
		JOIN share_users su ON su.share_id = s.id
		WHERE s.target_type = 'file' AND s.target_id = $1 AND s.revoked = false AND su.target_user_id = $2
		LIMIT 1
	`, fileID, userID).Scan(&hasDirect)

	allowed := derr == nil
	if !allowed {
		// Check folder share via ancestors of the file's folder
		var hasFolder int
		ferr := h.db.QueryRowContext(c.Request.Context(), `
			WITH RECURSIVE ancestors AS (
				SELECT id, parent_id FROM folders WHERE id = NULLIF($1, '')::uuid
				UNION ALL
				SELECT f.id, f.parent_id FROM folders f
				JOIN ancestors a ON f.id = a.parent_id
			)
			SELECT 1
			FROM shares s
			JOIN share_users su ON su.share_id = s.id
			WHERE su.target_user_id = $2
			  AND s.target_type = 'folder'
			  AND s.revoked = false
			  AND s.target_id IN (SELECT id FROM ancestors)
			LIMIT 1
		`, folderID, userID).Scan(&hasFolder)
		allowed = ferr == nil
	}

	if !allowed {
		c.JSON(http.StatusForbidden, gin.H{"error": "not authorized"})
		return
	}

	url, perr := h.storage.PresignedGetURL(c.Request.Context(), blobKey, 15)
	if perr != nil {
		logger.L.Error("shares.downloadSharedFile: presign failed", zap.Error(perr))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"downloadUrl": url, "filename": filename})
}

// listSharedFolderAncestors returns the ancestor chain for a shared folder (root->current) if the user has access via a share.
func (h *shareHandler) listSharedFolderAncestors(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	folderID := c.Param("id")

	// Verify access via share (folder share on ancestor or self)
	var ok int
	if err := h.db.QueryRowContext(c.Request.Context(), `
		WITH RECURSIVE ancestors AS (
			SELECT id FROM folders WHERE id = $1
			UNION ALL
			SELECT f.parent_id FROM folders f JOIN ancestors a ON f.id = a.id
		)
		SELECT 1 FROM shares s JOIN share_users su ON su.share_id = s.id
		WHERE su.target_user_id = $2 AND s.target_type='folder' AND s.revoked=false AND s.target_id IN (SELECT id FROM ancestors) LIMIT 1
	`, folderID, userID).Scan(&ok); err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusForbidden, gin.H{"error": "not authorized"})
			return
		}
		logger.L.Error("shares.listSharedFolderAncestors: auth check failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}

	// Collect ancestors by walking up parent_id
	rows, err := h.db.QueryContext(c.Request.Context(), `
		WITH RECURSIVE anc AS (
			SELECT id, parent_id, name FROM folders WHERE id = $1
			UNION ALL
			SELECT f.id, f.parent_id, f.name FROM folders f JOIN anc a ON f.id = a.parent_id
		)
		SELECT id, name FROM anc
	`, folderID)
	if err != nil {
		logger.L.Error("shares.listSharedFolderAncestors: query failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	defer rows.Close()

	type aRow struct{ id, name string }
	arr := []aRow{}
	for rows.Next() {
		var a aRow
		if err := rows.Scan(&a.id, &a.name); err == nil {
			arr = append(arr, a)
		}
	}
	// reverse to root->current
	for i, j := 0, len(arr)-1; i < j; i, j = i+1, j-1 {
		arr[i], arr[j] = arr[j], arr[i]
	}

	out := make([]map[string]string, 0, len(arr))
	for _, v := range arr {
		out = append(out, map[string]string{"id": v.id, "name": v.name})
	}
	c.JSON(http.StatusOK, gin.H{"ancestors": out})
}

// listSharedWithMe returns files and folders shared with the authenticated user
func (h *shareHandler) listSharedWithMe(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}

	// Pagination params for files and folders independently (optional)
	// For simplicity, reusing limit/offset for both
	limit := 50
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			if n > 0 && n <= 200 {
				limit = n
			} else if n > 200 {
				limit = 200
			}
		}
	}
	offset := 0
	if v := c.Query("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	// Files shared with the user
	filesRows, err := h.db.QueryContext(c.Request.Context(), `
		SELECT DISTINCT ON (uf.id)
			   uf.id, uf.filename, uf.declared_mime, uf.original_size_bytes,
			   uf.created_at, uf.updated_at, uf.download_count, u.email
		FROM user_files uf
		JOIN users u ON u.id = uf.user_id
		JOIN shares s ON s.target_type='file' AND s.target_id = uf.id AND s.revoked = false
		JOIN share_users su ON su.share_id = s.id
		WHERE su.target_user_id = $1 AND uf.deleted_at IS NULL
		ORDER BY uf.id, uf.created_at DESC
		LIMIT $2 OFFSET $3
	`, userID, limit, offset)
	if err != nil {
		logger.L.Error("query shared files failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	defer filesRows.Close()

	sharedFiles := make([]map[string]interface{}, 0)
	for filesRows.Next() {
		var (
			id, filename, mime, ownerEmail string
			size                 int64
			createdAt, updatedAt time.Time
			downloadCount        int64
		)
		if err := filesRows.Scan(&id, &filename, &mime, &size, &createdAt, &updatedAt, &downloadCount, &ownerEmail); err == nil {
			sharedFiles = append(sharedFiles, map[string]interface{}{
				"id":            id,
				"filename":      filename,
				"mime":          mime,
				"size":          size,
				"createdAt":     createdAt,
				"updatedAt":     updatedAt,
				"downloadCount": downloadCount,
				"ownerEmail":    ownerEmail,
			})
		}
	}

	// Folders shared with the user
	folderRows, err := h.db.QueryContext(c.Request.Context(), `
		SELECT DISTINCT ON (f.id)
			   f.id, f.name, f.created_at, f.updated_at, u.email,
			   COALESCE((
				   SELECT SUM(fc.size_bytes)
				   FROM user_files uf2
				   JOIN file_contents fc ON uf2.content_id = fc.id
				   WHERE uf2.folder_id = f.id AND uf2.deleted_at IS NULL
			   ), 0) as size
		FROM folders f
		JOIN users u ON u.id = f.user_id
		JOIN shares s ON s.target_type='folder' AND s.target_id = f.id AND s.revoked = false
		JOIN share_users su ON su.share_id = s.id
		WHERE su.target_user_id = $1 AND f.deleted_at IS NULL
		ORDER BY f.id, f.created_at DESC
		LIMIT $2 OFFSET $3
	`, userID, limit, offset)
	if err != nil {
		logger.L.Error("query shared folders failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	defer folderRows.Close()

	sharedFolders := make([]map[string]interface{}, 0)
	for folderRows.Next() {
		var (
			id, name, ownerEmail string
			createdAt, updatedAt time.Time
			size                 int64
		)
		if err := folderRows.Scan(&id, &name, &createdAt, &updatedAt, &ownerEmail, &size); err == nil {
			sharedFolders = append(sharedFolders, map[string]interface{}{
				"id":         id,
				"name":       name,
				"createdAt":  createdAt,
				"updatedAt":  updatedAt,
				"ownerEmail": ownerEmail,
				"size":       size,
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"files":   sharedFiles,
		"folders": sharedFolders,
		"limit":   limit,
		"offset":  offset,
	})
}

// GET /s/:token
// No auth required. If token points to file -> return metadata + presigned download url(s).
// If token points to folder -> return folder listing + presigned urls for files (first N).
func resolveShareHandler(db *sql.DB, st *storage.MinioStorage, c cache.Cache) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		token := ctx.Param("token")
		cacheKey := cache.ShareResolveKey(token)

		// try cache
		if c != nil {
			var cachedResp map[string]interface{}
			if err := c.Get(ctx.Request.Context(), cacheKey, &cachedResp); err == nil {
				ctx.JSON(http.StatusOK, cachedResp)
				return
			}
		}

		var id, targetType, targetId string
		var expires sql.NullTime
		err := db.QueryRowContext(ctx.Request.Context(), `
			SELECT id, target_type, target_id, expires_at
			FROM shares
			WHERE token=$1 AND revoked=false
		`, token).Scan(&id, &targetType, &targetId, &expires)
		if err != nil {
			if err == sql.ErrNoRows {
				ctx.JSON(http.StatusNotFound, gin.H{"error": "share not found"})
				return
			}
			logger.L.Error("db error", zap.Error(err))
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
			return
		}
		if expires.Valid && expires.Time.Before(time.Now()) {
			ctx.JSON(http.StatusGone, gin.H{"error": "share expired"})
			return
		}

		resp := gin.H{}
		switch targetType {
		case "file":
			var ufid, filename, blobKey string
			var size int64
			err := db.QueryRowContext(ctx.Request.Context(), `
				SELECT uf.id, uf.filename, fc.blob_key, fc.size_bytes
				FROM user_files uf
				JOIN file_contents fc ON uf.content_id = fc.id
				WHERE uf.id=$1 AND uf.deleted_at IS NULL
			`, targetId).Scan(&ufid, &filename, &blobKey, &size)
			if err != nil {
				if err == sql.ErrNoRows {
					ctx.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
					return
				}
				logger.L.Error("db error", zap.Error(err))
				ctx.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
				return
			}

			url, err := st.PresignedGetURL(ctx.Request.Context(), blobKey, 15)
			if err != nil {
				logger.L.Error("presign failed", zap.Error(err))
				ctx.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
				return
			}

			resp = gin.H{
				"type":     "file",
				"fileId":   ufid,
				"filename": filename,
				"size":     size,
				"download": url,
			}

		case "folder":
			files := []map[string]interface{}{}
			rows, err := db.QueryContext(ctx.Request.Context(), `
				SELECT uf.id, uf.filename, fc.blob_key, fc.size_bytes
				FROM user_files uf
				JOIN file_contents fc ON uf.content_id = fc.id
				WHERE uf.folder_id = $1 AND uf.deleted_at IS NULL
				LIMIT 100
			`, targetId)
			if err != nil {
				logger.L.Error("db err", zap.Error(err))
				ctx.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
				return
			}
			defer rows.Close()

			for rows.Next() {
				var fid, fname, blob string
				var fsize int64
				_ = rows.Scan(&fid, &fname, &blob, &fsize)
				url, _ := st.PresignedGetURL(ctx.Request.Context(), blob, 15)
				files = append(files, map[string]interface{}{
					"id":       fid,
					"filename": fname,
					"size":     fsize,
					"download": url,
				})
			}
			resp = gin.H{"type": "folder", "files": files}

		default:
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "unknown share target"})
			return
		}

		// set cache
		if c != nil {
			ttl := 5 * time.Minute
			if expires.Valid {
				expTtl := time.Until(expires.Time)
				if expTtl < ttl {
					ttl = expTtl
				}
			}
			_ = c.Set(ctx.Request.Context(), cacheKey, resp, ttl)
		}

		ctx.JSON(http.StatusOK, resp)
	}
}
