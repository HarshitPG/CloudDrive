package rest

import (
	"database/sql"
	"net/http"
	"time"

	"backend/internal/auth"
	"backend/internal/cache"
	sharesvc "backend/internal/share"
	"backend/internal/storage"
	"backend/internal/worker"
	"backend/pkg/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type shareHandler struct {
	db       *sql.DB
	storage  *storage.MinioStorage
	cache    cache.Cache
	producer *worker.Producer
}

func RegisterShareRoutes(rg *gin.RouterGroup, db *sql.DB, st *storage.MinioStorage, jwtSecret string, c cache.Cache) {
	h := &shareHandler{
		db:       db,
		storage:  st,
		cache:    c,
		producer: worker.NewProducer(),
	}

	// Public share routes (no auth required)
	rg.GET("/s/:token", h.resolveShare)
	rg.GET("/fs/:token", h.resolveFolderShare)
	rg.GET("/fs/:token/contents", h.resolveFolderShareContents)
	rg.GET("/fs/:token/ancestors", h.resolveFolderShareAncestors)
	rg.GET("/fs/:token/download/:fileId", h.downloadFromShare)
	rg.GET("/fs/:token/download", h.downloadFolderArchive)

	// Protected share routes (auth required)
	protected := rg.Group("/shares")
	protected.Use(auth.RequireAuth(jwtSecret))
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

	// Advanced folder share routes
	folderProtected := rg.Group("/folder-shares")
	folderProtected.Use(auth.RequireAuth(jwtSecret))
	folderProtected.POST("", h.createFolderShare)
	folderProtected.DELETE("/:id", h.revokeShare)
	folderProtected.GET("/:id", h.getShareInfo)
}

// resolveShare godoc
//
//	@Summary		Resolve public file share
//	@Description	Get details of a publicly shared file using share token
//	@Tags			shares
//	@Accept			json
//	@Produce		json
//	@Param			token	path		string	true	"Share token"
//	@Success		200		{object}	map[string]interface{}	"Share details with download URL"
//	@Failure		404		{object}	map[string]interface{}	"Share not found"
//	@Failure		410		{object}	map[string]interface{}	"Share expired"
//	@Router			/api/v1/s/{token} [get]
func (h *shareHandler) resolveShare(ctx *gin.Context) {
	token := ctx.Param("token")
	cacheKey := cache.ShareResolveKey(token)

	if h.cache != nil {
		var cachedResp map[string]interface{}
		if err := h.cache.Get(ctx.Request.Context(), cacheKey, &cachedResp); err == nil {
			ctx.JSON(http.StatusOK, cachedResp)
			return
		}
	}

	var id, targetType, targetId string
	var expires sql.NullTime
	err := h.db.QueryRowContext(ctx.Request.Context(), `
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
		err := h.db.QueryRowContext(ctx.Request.Context(), `
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
		url, err := h.storage.PresignedGetURL(ctx.Request.Context(), blobKey, 15)
		if err != nil {
			logger.L.Error("presign failed", zap.Error(err))
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
			return
		}
		resp = gin.H{"type": "file", "fileId": ufid, "filename": filename, "size": size, "download": url}
	case "folder":
		files := []map[string]interface{}{}
		rows, err := h.db.QueryContext(ctx.Request.Context(), `
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
			url, _ := h.storage.PresignedGetURL(ctx.Request.Context(), blob, 15)
			files = append(files, map[string]interface{}{"id": fid, "filename": fname, "size": fsize, "download": url})
		}
		resp = gin.H{"type": "folder", "files": files}
	default:
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "unknown share target"})
		return
	}

	if h.cache != nil {
		ttl := 5 * time.Minute
		if expires.Valid {
			if expTtl := time.Until(expires.Time); expTtl < ttl {
				ttl = expTtl
			}
		}
		_ = h.cache.Set(ctx.Request.Context(), cacheKey, resp, ttl)
	}

	ctx.JSON(http.StatusOK, resp)
}

// resolveFolderShare godoc
//
//	@Summary		Resolve public folder share
//	@Description	Get details and contents of a publicly shared folder using share token
//	@Tags			shares
//	@Accept			json
//	@Produce		json
//	@Param			token	path		string	true	"Folder share token"
//	@Success		200		{object}	map[string]interface{}	"Folder share details with items"
//	@Failure		404		{object}	map[string]interface{}	"Share not found"
//	@Failure		410		{object}	map[string]interface{}	"Share expired"
//	@Router			/api/v1/fs/{token} [get]
func (h *shareHandler) resolveFolderShare(c *gin.Context) {
	// Use the existing implementation from share package
	handler := &sharesvc.Handler{DB: h.db, Storage: h.storage, Cache: h.cache, Producer: h.producer}
	handler.ResolveFolderShare()(c)
}

// resolveFolderShareContents godoc
//
//	@Summary		Get folder share contents
//	@Description	Get the contents of a specific folder within a folder share
//	@Tags			shares
//	@Accept			json
//	@Produce		json
//	@Param			token		path		string	true	"Folder share token"
//	@Param			folderId	query		string	false	"Specific folder ID to list (defaults to root shared folder)"
//	@Success		200			{object}	map[string]interface{}	"Folder contents"
//	@Failure		403			{object}	map[string]interface{}	"Forbidden"
//	@Failure		404			{object}	map[string]interface{}	"Share not found"
//	@Failure		410			{object}	map[string]interface{}	"Share expired"
//	@Router			/api/v1/fs/{token}/contents [get]
func (h *shareHandler) resolveFolderShareContents(c *gin.Context) {
	handler := &sharesvc.Handler{DB: h.db, Storage: h.storage, Cache: h.cache, Producer: h.producer}
	handler.ResolveFolderShareContents()(c)
}

// resolveFolderShareAncestors godoc
//
//	@Summary		Get folder share ancestors
//	@Description	Get the ancestor folders (breadcrumb path) for a folder within a share
//	@Tags			shares
//	@Accept			json
//	@Produce		json
//	@Param			token		path		string	true	"Folder share token"
//	@Param			folderId	query		string	false	"Folder ID to get ancestors for"
//	@Success		200			{object}	map[string]interface{}	"List of ancestor folders"
//	@Failure		403			{object}	map[string]interface{}	"Forbidden"
//	@Failure		404			{object}	map[string]interface{}	"Share not found"
//	@Failure		410			{object}	map[string]interface{}	"Share expired"
//	@Router			/api/v1/fs/{token}/ancestors [get]
func (h *shareHandler) resolveFolderShareAncestors(c *gin.Context) {
	handler := &sharesvc.Handler{DB: h.db, Storage: h.storage, Cache: h.cache, Producer: h.producer}
	handler.ResolveFolderShareAncestors()(c)
}

// downloadFromShare godoc
//
//	@Summary		Download file from folder share
//	@Description	Get a download URL for a specific file within a folder share
//	@Tags			shares
//	@Accept			json
//	@Produce		json
//	@Param			token	path		string	true	"Folder share token"
//	@Param			fileId	path		string	true	"File ID to download"
//	@Success		200		{object}	map[string]interface{}	"Download URL and file info"
//	@Failure		403		{object}	map[string]interface{}	"File not accessible through this share"
//	@Failure		404		{object}	map[string]interface{}	"Share or file not found"
//	@Failure		410		{object}	map[string]interface{}	"Share expired"
//	@Router			/api/v1/fs/{token}/download/{fileId} [get]
func (h *shareHandler) downloadFromShare(c *gin.Context) {
	handler := &sharesvc.Handler{DB: h.db, Storage: h.storage, Cache: h.cache, Producer: h.producer}
	handler.DownloadFromShare()(c)
}

// downloadFolderArchive godoc
//
//	@Summary		Download folder share as ZIP archive
//	@Description	Download all files in a folder share as a ZIP archive
//	@Tags			shares
//	@Accept			json
//	@Produce		application/zip
//	@Param			token	path	string	true	"Folder share token"
//	@Success		200		{file}	binary	"ZIP archive of folder contents"
//	@Failure		404		{object}	map[string]interface{}	"Share not found"
//	@Failure		410		{object}	map[string]interface{}	"Share expired"
//	@Router			/api/v1/fs/{token}/download [get]
func (h *shareHandler) downloadFolderArchive(c *gin.Context) {
	handler := &sharesvc.Handler{DB: h.db, Storage: h.storage, Cache: h.cache, Producer: h.producer}
	handler.DownloadFolderArchive()(c)
}

// createPublicFileShare godoc
//
//	@Summary		Create public file share
//	@Description	Create a public share link for a file
//	@Tags			shares
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string	true	"File ID"
//	@Param			body	body		map[string]interface{}	false	"Share options (title, description, expiresAt)"
//	@Success		201		{object}	map[string]interface{}	"Share created with token and URL"
//	@Failure		403		{object}	map[string]interface{}	"Not the file owner"
//	@Failure		404		{object}	map[string]interface{}	"File not found"
//	@Security		BearerAuth
//	@Router			/api/v1/shares/files/{id}/share [post]
func (h *shareHandler) createPublicFileShare(c *gin.Context) {
	handler := &sharesvc.Handler{DB: h.db, Storage: h.storage, Cache: h.cache, Producer: h.producer}
	handler.CreatePublicFileShare()(c)
}

// downloadSharedFile godoc
//
//	@Summary		Download shared file
//	@Description	Get download URL for a file shared with the authenticated user
//	@Tags			shares
//	@Accept			json
//	@Produce		json
//	@Param			id	path		string	true	"File ID"
//	@Success		200	{object}	map[string]interface{}	"Download URL"
//	@Failure		403	{object}	map[string]interface{}	"Access denied"
//	@Failure		404	{object}	map[string]interface{}	"File not found"
//	@Security		BearerAuth
//	@Router			/api/v1/shares/files/{id}/download [get]
func (h *shareHandler) downloadSharedFile(c *gin.Context) {
	handler := &sharesvc.Handler{DB: h.db, Storage: h.storage, Cache: h.cache, Producer: h.producer}
	handler.DownloadSharedFile()(c)
}

// revokeShare godoc
//
//	@Summary		Revoke share
//	@Description	Revoke a file or folder share (creator only)
//	@Tags			shares
//	@Accept			json
//	@Produce		json
//	@Param			id	path		string	true	"Share ID"
//	@Success		200	{object}	map[string]interface{}	"Share revoked"
//	@Failure		403	{object}	map[string]interface{}	"Not the share creator"
//	@Failure		404	{object}	map[string]interface{}	"Share not found"
//	@Security		BearerAuth
//	@Router			/api/v1/shares/shares/{id} [delete]
func (h *shareHandler) revokeShare(c *gin.Context) {
	handler := &sharesvc.Handler{DB: h.db, Storage: h.storage, Cache: h.cache, Producer: h.producer}
	handler.RevokeShare()(c)
}

// shareFileToUser godoc
//
//	@Summary		Share file with specific user
//	@Description	Share a file with a specific user by email with specified permissions
//	@Tags			shares
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string	true	"File ID"
//	@Param			body	body		map[string]interface{}	true	"Target user email and permission (read/write)"
//	@Success		201		{object}	map[string]interface{}	"Share created"
//	@Failure		403		{object}	map[string]interface{}	"Not the file owner"
//	@Failure		404		{object}	map[string]interface{}	"File or target user not found"
//	@Security		BearerAuth
//	@Router			/api/v1/shares/files/{id}/share/user [post]
func (h *shareHandler) shareFileToUser(c *gin.Context) {
	handler := &sharesvc.Handler{DB: h.db, Storage: h.storage, Cache: h.cache, Producer: h.producer}
	handler.ShareFileToUser()(c)
}

// listFileShares godoc
//
//	@Summary		List file shares
//	@Description	List all shares (public and user-specific) for a file
//	@Tags			shares
//	@Accept			json
//	@Produce		json
//	@Param			id	path		string	true	"File ID"
//	@Success		200	{object}	map[string]interface{}	"List of public and user shares"
//	@Failure		403	{object}	map[string]interface{}	"Not the file owner"
//	@Failure		404	{object}	map[string]interface{}	"File not found"
//	@Security		BearerAuth
//	@Router			/api/v1/shares/files/{id}/shares [get]
func (h *shareHandler) listFileShares(c *gin.Context) {
	handler := &sharesvc.Handler{DB: h.db, Storage: h.storage, Cache: h.cache, Producer: h.producer}
	handler.ListFileShares()(c)
}

// createPublicFolderShare godoc
//
//	@Summary		Create public folder share
//	@Description	Create a public share link for a folder
//	@Tags			shares
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string	true	"Folder ID"
//	@Param			body	body		map[string]interface{}	false	"Share options (title, description, expiresAt)"
//	@Success		201		{object}	map[string]interface{}	"Share created with token and URL"
//	@Failure		403		{object}	map[string]interface{}	"Not the folder owner"
//	@Failure		404		{object}	map[string]interface{}	"Folder not found"
//	@Security		BearerAuth
//	@Router			/api/v1/shares/folders/{id}/share [post]
func (h *shareHandler) createPublicFolderShare(c *gin.Context) {
	handler := &sharesvc.Handler{DB: h.db, Storage: h.storage, Cache: h.cache, Producer: h.producer}
	handler.CreatePublicFolderShare()(c)
}

// shareFolderToUser godoc
//
//	@Summary		Share folder with specific user
//	@Description	Share a folder with a specific user by email with specified permissions
//	@Tags			shares
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string	true	"Folder ID"
//	@Param			body	body		map[string]interface{}	true	"Target user email and permission (read/write)"
//	@Success		201		{object}	map[string]interface{}	"Share created"
//	@Failure		403		{object}	map[string]interface{}	"Not the folder owner"
//	@Failure		404		{object}	map[string]interface{}	"Folder or target user not found"
//	@Security		BearerAuth
//	@Router			/api/v1/shares/folders/{id}/share/user [post]
func (h *shareHandler) shareFolderToUser(c *gin.Context) {
	handler := &sharesvc.Handler{DB: h.db, Storage: h.storage, Cache: h.cache, Producer: h.producer}
	handler.ShareFolderToUser()(c)
}

// listSharedFolderContents godoc
//
//	@Summary		List contents of shared folder
//	@Description	List files and subfolders in a folder shared with the authenticated user
//	@Tags			shares
//	@Accept			json
//	@Produce		json
//	@Param			id	path		string	true	"Folder ID"
//	@Success		200	{object}	map[string]interface{}	"Folder contents"
//	@Failure		403	{object}	map[string]interface{}	"Access denied"
//	@Failure		404	{object}	map[string]interface{}	"Folder not found"
//	@Security		BearerAuth
//	@Router			/api/v1/shares/folders/{id}/contents [get]
func (h *shareHandler) listSharedFolderContents(c *gin.Context) {
	handler := &sharesvc.Handler{DB: h.db, Storage: h.storage, Cache: h.cache, Producer: h.producer}
	handler.ListSharedFolderContents()(c)
}

// listSharedFolderAncestors godoc
//
//	@Summary		List ancestors of shared folder
//	@Description	Get the parent folder hierarchy for a folder shared with the authenticated user
//	@Tags			shares
//	@Accept			json
//	@Produce		json
//	@Param			id	path		string	true	"Folder ID"
//	@Success		200	{object}	map[string]interface{}	"List of ancestor folders"
//	@Failure		403	{object}	map[string]interface{}	"Access denied"
//	@Failure		404	{object}	map[string]interface{}	"Folder not found"
//	@Security		BearerAuth
//	@Router			/api/v1/shares/folders/{id}/ancestors [get]
func (h *shareHandler) listSharedFolderAncestors(c *gin.Context) {
	handler := &sharesvc.Handler{DB: h.db, Storage: h.storage, Cache: h.cache, Producer: h.producer}
	handler.ListSharedFolderAncestors()(c)
}

// listSharedWithMe godoc
//
//	@Summary		List items shared with me
//	@Description	Get all files and folders that have been shared with the authenticated user
//	@Tags			shares
//	@Accept			json
//	@Produce		json
//	@Param			limit	query		int	false	"Number of items per page (default 50)"
//	@Param			offset	query		int	false	"Pagination offset (default 0)"
//	@Success		200		{object}	map[string]interface{}	"List of shared files and folders"
//	@Security		BearerAuth
//	@Router			/api/v1/shares/shared-with-me [get]
func (h *shareHandler) listSharedWithMe(c *gin.Context) {
	handler := &sharesvc.Handler{DB: h.db, Storage: h.storage, Cache: h.cache, Producer: h.producer}
	handler.ListSharedWithMe()(c)
}

// createFolderShare godoc
//
//	@Summary		Create advanced folder share
//	@Description	Create a folder share with advanced options (recursive, snapshot mode, etc.)
//	@Tags			shares
//	@Accept			json
//	@Produce		json
//	@Param			body	body		map[string]interface{}	true	"Folder share configuration"
//	@Success		201		{object}	map[string]interface{}	"Folder share created"
//	@Failure		403		{object}	map[string]interface{}	"Not the folder owner"
//	@Failure		404		{object}	map[string]interface{}	"Folder not found"
//	@Security		BearerAuth
//	@Router			/api/v1/folder-shares [post]
func (h *shareHandler) createFolderShare(c *gin.Context) {
	handler := &sharesvc.Handler{DB: h.db, Storage: h.storage, Cache: h.cache, Producer: h.producer}
	handler.CreateFolderShare()(c)
}

// getShareInfo godoc
//
//	@Summary		Get folder share info
//	@Description	Get detailed information about a folder share (creator only)
//	@Tags			shares
//	@Accept			json
//	@Produce		json
//	@Param			id	path		string	true	"Folder share ID"
//	@Success		200	{object}	map[string]interface{}	"Share details"
//	@Failure		403	{object}	map[string]interface{}	"Not the share creator"
//	@Failure		404	{object}	map[string]interface{}	"Share not found"
//	@Security		BearerAuth
//	@Router			/api/v1/folder-shares/{id} [get]
func (h *shareHandler) getShareInfo(c *gin.Context) {
	handler := &sharesvc.Handler{DB: h.db, Storage: h.storage, Cache: h.cache, Producer: h.producer}
	handler.GetShareInfo()(c)
}
