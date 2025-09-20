package rest

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"time"

	"backend/internal/auth"
	"backend/internal/cache"
	"backend/internal/storage"
	"backend/internal/worker"
	"backend/pkg/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const (
	maxFoldersPerQuery = 500
	maxFilesPerQuery   = 1000
	maxDirectItems     = 100
	presignedURLTTL    = 15 * time.Minute
	cacheTTL           = 2 * time.Minute
)

// RegisterFolderShareRoutes registers the folder share API routes
func RegisterFolderShareRoutes(rg *gin.RouterGroup, db *sql.DB, st *storage.MinioStorage, jwtSecret string, c cache.Cache) {
	// Public routes
	rg.GET("/fs/:token", resolveFolderShareHandler(db, st, c))
	rg.GET("/fs/:token/download/:fileId", downloadFromShareHandler(db, st, c))
	rg.GET("/fs/:token/download", downloadFolderArchiveHandler(db, st, c))

	// Protected routes
	protected := rg.Group("/folder-shares")
	protected.Use(auth.RequireAuth(jwtSecret))
	h := &folderShareHandler{
		db:       db,
		storage:  st,
		cache:    c,
		producer: worker.NewProducer(),
	}

	protected.POST("", h.createFolderShare)
	protected.DELETE("/:id", h.revokeShare)
	protected.GET("/:id", h.getShareInfo)
}

type folderShareHandler struct {
	db       *sql.DB
	storage  *storage.MinioStorage
	cache    cache.Cache
	producer *worker.Producer
}

type createFolderShareReq struct {
	FolderID     string     `json:"folderId" binding:"required"`
	Title        string     `json:"title"`
	Description  string     `json:"description"`
	Recursive    bool       `json:"recursive"`
	SnapshotMode bool       `json:"snapshotMode"`
	ExpiresAt    *time.Time `json:"expiresAt"`
}

type folderShareResponse struct {
	ID           string     `json:"id"`
	Token        string     `json:"token"`
	URL          string     `json:"url"`
	FolderID     string     `json:"folderId"`
	FolderName   string     `json:"folderName"`
	Title        string     `json:"title"`
	Description  string     `json:"description"`
	Recursive    bool       `json:"recursive"`
	SnapshotMode bool       `json:"snapshotMode"`
	ExpiresAt    *time.Time `json:"expiresAt"`
	CreatedAt    time.Time  `json:"createdAt"`
}

type shareItem struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Type     string  `json:"type"` // "file" | "folder"
	Size     *int64  `json:"size,omitempty"`
	MimeType string  `json:"mimeType,omitempty"`
	Path     string  `json:"path"`
	ParentID *string `json:"parentId,omitempty"`
}

type folderShareListing struct {
	Share   folderShareResponse `json:"share"`
	Items   []shareItem         `json:"items"`
	Total   int                 `json:"total"`
	HasMore bool                `json:"hasMore"`
}

func generateShareToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func (h *folderShareHandler) createFolderShare(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())

	var req createFolderShareReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request payload"})
		return
	}

	// Validate folder ownership
	folderName, err := h.validateFolderOwnership(c.Request.Context(), req.FolderID, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "folder not found"})
			return
		}
		logger.L.Error("folder validation failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	// Generate token
	token, err := generateShareToken()
	if err != nil {
		logger.L.Error("token generation failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	// Create share
	shareID, createdAt, err := h.createShare(c.Request.Context(), token, userID, req)
	if err != nil {
		logger.L.Error("share creation failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	// Handle snapshot mode asynchronously
	if req.SnapshotMode && req.Recursive {
		h.enqueueSnapshotJob(c.Request.Context(), shareID, req.FolderID, token, createdAt)
	}

	// Publish creation event
	h.publishShareEvent(c.Request.Context(), "created", shareID, req.FolderID, token, userID, "", createdAt)

	// Invalidate cache
	h.invalidateShareCache(c.Request.Context(), token)

	response := folderShareResponse{
		ID:           shareID,
		Token:        token,
		URL:          "/fs/" + token,
		FolderID:     req.FolderID,
		FolderName:   folderName,
		Title:        req.Title,
		Description:  req.Description,
		Recursive:    req.Recursive,
		SnapshotMode: req.SnapshotMode,
		ExpiresAt:    req.ExpiresAt,
		CreatedAt:    createdAt,
	}

	c.JSON(http.StatusCreated, response)
}

func (h *folderShareHandler) validateFolderOwnership(ctx context.Context, folderID, userID string) (string, error) {
	var folderName, owner string
	err := h.db.QueryRowContext(ctx,
		"SELECT user_id, name FROM folders WHERE id=$1 AND deleted_at IS NULL",
		folderID).Scan(&owner, &folderName)

	if err != nil {
		return "", err
	}

	if owner != userID {
		return "", fmt.Errorf("unauthorized access to folder")
	}

	return folderName, nil
}

func (h *folderShareHandler) createShare(ctx context.Context, token, userID string, req createFolderShareReq) (string, time.Time, error) {
	var shareID string
	var createdAt time.Time

	err := h.db.QueryRowContext(ctx, `
		INSERT INTO shares (id, token, creator_id, target_type, target_id, title, description, 
			expires_at, recursive, snapshot_mode, created_at)
		VALUES (gen_random_uuid(), $1, $2, 'folder', $3, $4, $5, $6, $7, $8, now())
		RETURNING id, created_at
	`, token, userID, req.FolderID, req.Title, req.Description, req.ExpiresAt, req.Recursive, req.SnapshotMode).Scan(&shareID, &createdAt)

	return shareID, createdAt, err
}

func (h *folderShareHandler) enqueueSnapshotJob(ctx context.Context, shareID, folderID, token string, createdAt time.Time) {
	job := worker.FolderShareJob{
		ShareID:      shareID,
		FolderID:     folderID,
		Token:        token,
		Recursive:    true,
		SnapshotMode: true,
		CreatedAt:    createdAt.Format(time.RFC3339),
	}

	if err := h.producer.PublishFolderShareJob(ctx, job); err != nil {
		logger.L.Warn("failed to enqueue snapshot job", zap.Error(err))
	}
}

func (h *folderShareHandler) publishShareEvent(ctx context.Context, eventType, shareID, folderID, token, userID, ipAddress string, timestamp time.Time) {
	event := worker.FolderShareEvent{
		Type:      eventType,
		ShareID:   shareID,
		FolderID:  folderID,
		Token:     token,
		UserID:    userID,
		IPAddress: ipAddress,
		Timestamp: timestamp.Format(time.RFC3339),
	}

	if err := h.producer.PublishFolderShareEvent(ctx, event); err != nil {
		logger.L.Warn("failed to publish share event", zap.String("eventType", eventType), zap.Error(err))
	}
}

func (h *folderShareHandler) invalidateShareCache(ctx context.Context, token string) {
	if h.cache != nil {
		cache.InvalidateShare(ctx, h.cache, token)
	}
}

func (h *folderShareHandler) revokeShare(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	shareID := c.Param("id")

	if shareID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "share ID is required"})
		return
	}

	// Verify ownership and get token
	creatorID, token, err := h.getShareOwnership(c.Request.Context(), shareID)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "share not found"})
			return
		}
		logger.L.Error("share ownership check failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	if creatorID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "not authorized"})
		return
	}

	// Revoke share
	if err := h.revokeShareInDB(c.Request.Context(), shareID); err != nil {
		logger.L.Error("share revocation failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	// Publish revocation event asynchronously
	go h.publishShareEvent(context.Background(), "revoked", shareID, "", token, userID, "", time.Now())

	// Invalidate cache
	h.invalidateShareCache(c.Request.Context(), token)

	c.JSON(http.StatusOK, gin.H{"message": "share revoked"})
}

func (h *folderShareHandler) getShareOwnership(ctx context.Context, shareID string) (string, string, error) {
	var creatorID, token string
	err := h.db.QueryRowContext(ctx,
		"SELECT creator_id, token FROM shares WHERE id=$1", shareID).Scan(&creatorID, &token)
	return creatorID, token, err
}

func (h *folderShareHandler) revokeShareInDB(ctx context.Context, shareID string) error {
	_, err := h.db.ExecContext(ctx, "UPDATE shares SET revoked=true WHERE id=$1", shareID)
	return err
}

func (h *folderShareHandler) getShareInfo(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	shareID := c.Param("id")

	if shareID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "share ID is required"})
		return
	}

	share, err := h.getShareByID(c.Request.Context(), shareID)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "share not found"})
			return
		}
		logger.L.Error("share lookup failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	if share.CreatorID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "not authorized"})
		return
	}

	response := h.buildShareResponse(share)
	c.JSON(http.StatusOK, response)
}

func (h *folderShareHandler) getShareByID(ctx context.Context, shareID string) (*folderShareRecord, error) {
	var share folderShareRecord
	var expiresAt sql.NullTime

	err := h.db.QueryRowContext(ctx, `
		SELECT s.id, s.token, s.creator_id, s.target_id, f.name, s.title, s.description,
			   s.recursive, s.snapshot_mode, s.expires_at, s.created_at
		FROM shares s
		JOIN folders f ON s.target_id = f.id
		WHERE s.id=$1 AND s.target_type='folder' AND s.revoked=false
	`, shareID).Scan(
		&share.ID, &share.Token, &share.CreatorID, &share.FolderID, &share.FolderName,
		&share.Title, &share.Description, &share.Recursive, &share.SnapshotMode, &expiresAt, &share.CreatedAt,
	)

	if err != nil {
		return nil, err
	}

	if expiresAt.Valid {
		share.ExpiresAt = &expiresAt.Time
	}

	return &share, nil
}

func (h *folderShareHandler) buildShareResponse(share *folderShareRecord) folderShareResponse {
	return folderShareResponse{
		ID:           share.ID,
		Token:        share.Token,
		FolderID:     share.FolderID,
		FolderName:   share.FolderName,
		Title:        share.Title,
		Description:  share.Description,
		Recursive:    share.Recursive,
		SnapshotMode: share.SnapshotMode,
		ExpiresAt:    share.ExpiresAt,
		CreatedAt:    share.CreatedAt,
		URL:          "/fs/" + share.Token,
	}
}

type folderShareRecord struct {
	ID           string
	Token        string
	CreatorID    string
	FolderID     string
	FolderName   string
	Title        string
	Description  string
	Recursive    bool
	SnapshotMode bool
	ExpiresAt    *time.Time
	CreatedAt    time.Time
}

// resolveFolderShareHandler resolves a folder share and returns listing
func resolveFolderShareHandler(db *sql.DB, st *storage.MinioStorage, c cache.Cache) gin.HandlerFunc {
	producer := worker.NewProducer()

	return func(ctx *gin.Context) {
		token := ctx.Param("token")

		// Try cache first
		cacheKey := cache.ShareResolveKey(token)
		if c != nil {
			var cached folderShareListing
			if err := c.Get(ctx.Request.Context(), cacheKey, &cached); err == nil {
				// Publish access event for cached response
				accessEvent := worker.FolderShareEvent{
					Type:      "accessed",
					ShareID:   cached.Share.ID,
					FolderID:  cached.Share.FolderID,
					Token:     token,
					IPAddress: ctx.ClientIP(),
					Timestamp: time.Now().Format(time.RFC3339),
				}
				_ = producer.PublishFolderShareEvent(ctx.Request.Context(), accessEvent)

				ctx.JSON(http.StatusOK, cached)
				return
			}
		}

		// Get share info
		var shareID, targetID, title, description string
		var recursive, snapshotMode bool
		var expiresAt sql.NullTime
		var createdAt time.Time

		err := db.QueryRowContext(ctx.Request.Context(), `
			SELECT id, target_id, title, description, recursive, snapshot_mode, expires_at, created_at
			FROM shares
			WHERE token=$1 AND target_type='folder' AND revoked=false
		`, token).Scan(&shareID, &targetID, &title, &description, &recursive, &snapshotMode, &expiresAt, &createdAt)

		if err != nil {
			if err == sql.ErrNoRows {
				ctx.JSON(http.StatusNotFound, gin.H{"error": "share not found"})
				return
			}
			logger.L.Error("db error", zap.Error(err))
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			return
		}

		// Check expiry
		if expiresAt.Valid && expiresAt.Time.Before(time.Now()) {
			ctx.JSON(http.StatusGone, gin.H{"error": "share expired"})
			return
		}

		// Get folder name
		var folderName string
		err = db.QueryRowContext(ctx.Request.Context(),
			"SELECT name FROM folders WHERE id=$1", targetID).Scan(&folderName)
		if err != nil {
			logger.L.Error("folder not found", zap.Error(err))
			ctx.JSON(http.StatusNotFound, gin.H{"error": "folder not found"})
			return
		}

		// Build response
		shareInfo := folderShareResponse{
			ID:           shareID,
			Token:        token,
			URL:          "/fs/" + token,
			FolderID:     targetID,
			FolderName:   folderName,
			Title:        title,
			Description:  description,
			Recursive:    recursive,
			SnapshotMode: snapshotMode,
			CreatedAt:    createdAt,
		}
		if expiresAt.Valid {
			shareInfo.ExpiresAt = &expiresAt.Time
		}

		// Get items
		var items []shareItem
		var total int

		// Get folder owner first
		var ownerID string
		err = db.QueryRowContext(ctx.Request.Context(), `
			SELECT user_id FROM folders WHERE id = $1 AND deleted_at IS NULL
		`, targetID).Scan(&ownerID)
		if err != nil {
			logger.L.Error("failed to get folder owner", zap.Error(err))
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			return
		}

		if recursive {
			// Use WITH RECURSIVE to get all nested items
			items, total = getRecursiveItemsStandalone(ctx.Request.Context(), db, targetID, ownerID)
		} else {
			// Get only direct children
			items, total = getDirectItemsStandalone(ctx.Request.Context(), db, targetID, ownerID)
		}

		listing := folderShareListing{
			Share:   shareInfo,
			Items:   items,
			Total:   total,
			HasMore: false, // TODO: Add pagination for large results
		}

		// Publish access event
		accessEvent := worker.FolderShareEvent{
			Type:      "accessed",
			ShareID:   shareID,
			FolderID:  targetID,
			Token:     token,
			IPAddress: ctx.ClientIP(),
			Timestamp: time.Now().Format(time.RFC3339),
		}
		_ = producer.PublishFolderShareEvent(ctx.Request.Context(), accessEvent)

		// Cache result
		if c != nil {
			ttl := 2 * time.Minute
			if expiresAt.Valid {
				remaining := time.Until(expiresAt.Time)
				if remaining < ttl {
					ttl = remaining
				}
			}
			_ = c.Set(ctx.Request.Context(), cacheKey, listing, ttl)
		}

		ctx.JSON(http.StatusOK, listing)
	}
}

// getDirectItems gets files and folders directly in the specified folder
func (h *folderShareHandler) getDirectItems(ctx context.Context, folderID, userID string) ([]shareItem, error) {
	logger.L.Info("getDirectItems called", zap.String("folderId", folderID), zap.String("userId", userID))

	var items []shareItem

	// Get subfolders
	if err := h.addSubfolders(ctx, &items, folderID, userID); err != nil {
		return nil, fmt.Errorf("failed to get subfolders: %w", err)
	}

	// Get files in the folder
	if err := h.addFiles(ctx, &items, folderID, userID); err != nil {
		return nil, fmt.Errorf("failed to get files: %w", err)
	}

	logger.L.Info("getDirectItems result",
		zap.String("folderId", folderID),
		zap.Int("totalItems", len(items)))

	return items, nil
}

func (h *folderShareHandler) getRecursiveItems(ctx context.Context, folderID, userID string) ([]shareItem, error) {
	logger.L.Info("getRecursiveItems called", zap.String("folderId", folderID), zap.String("userId", userID))

	var items []shareItem

	// Get all folders in the tree using WITH RECURSIVE
	if err := h.addRecursiveFolders(ctx, &items, folderID, userID); err != nil {
		return nil, fmt.Errorf("failed to get recursive folders: %w", err)
	}

	// Get all files in the tree
	if err := h.addRecursiveFiles(ctx, &items, folderID, userID); err != nil {
		return nil, fmt.Errorf("failed to get recursive files: %w", err)
	}

	logger.L.Info("getRecursiveItems result",
		zap.String("folderId", folderID),
		zap.Int("totalItems", len(items)))

	return items, nil
}

func (h *folderShareHandler) addSubfolders(ctx context.Context, items *[]shareItem, folderID, userID string) error {
	rows, err := h.db.QueryContext(ctx, `
		SELECT id, name, parent_id
		FROM folders
		WHERE parent_id = $1 AND user_id = $2 AND deleted_at IS NULL
		ORDER BY name
		LIMIT $3
	`, folderID, userID, maxFoldersPerQuery)

	if err != nil {
		return fmt.Errorf("subfolder query failed: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var item shareItem
		var parentID string
		if err := rows.Scan(&item.ID, &item.Name, &parentID); err != nil {
			logger.L.Error("failed to scan subfolder", zap.Error(err))
			continue
		}
		item.Type = "folder"
		item.Path = item.Name
		item.ParentID = &parentID
		*items = append(*items, item)
	}

	return rows.Err()
}

func (h *folderShareHandler) addFiles(ctx context.Context, items *[]shareItem, folderID, userID string) error {
	rows, err := h.db.QueryContext(ctx, `
		SELECT id, filename, original_size_bytes, declared_mime, folder_id
		FROM user_files
		WHERE folder_id = $1 AND user_id = $2 AND deleted_at IS NULL
		ORDER BY filename
		LIMIT $3
	`, folderID, userID, maxFilesPerQuery)

	if err != nil {
		return fmt.Errorf("file query failed: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var item shareItem
		var size int64
		var parentFolderID string
		if err := rows.Scan(&item.ID, &item.Name, &size, &item.MimeType, &parentFolderID); err != nil {
			logger.L.Error("failed to scan file", zap.Error(err))
			continue
		}
		item.Type = "file"
		item.Size = &size
		item.Path = item.Name
		item.ParentID = &parentFolderID
		*items = append(*items, item)
	}

	return rows.Err()
}

func (h *folderShareHandler) addRecursiveFolders(ctx context.Context, items *[]shareItem, folderID, userID string) error {
	rows, err := h.db.QueryContext(ctx, `
		WITH RECURSIVE folder_tree AS (
			-- Base case: start with the shared folder itself
			SELECT id, name, parent_id, name as folder_path
			FROM folders
			WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
			
			UNION ALL
			
			-- Recursive case: get children
			SELECT f.id, f.name, f.parent_id, 
				   CASE 
					   WHEN ft.folder_path = '' THEN f.name
					   ELSE ft.folder_path || '/' || f.name
				   END as folder_path
			FROM folders f
			INNER JOIN folder_tree ft ON f.parent_id = ft.id
			WHERE f.user_id = $2 AND f.deleted_at IS NULL
		)
		SELECT id, name, parent_id, folder_path
		FROM folder_tree
		WHERE id != $1  -- Exclude the root folder itself
		ORDER BY folder_path
		LIMIT $3
	`, folderID, userID, maxFoldersPerQuery)

	if err != nil {
		return fmt.Errorf("recursive folder query failed: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var item shareItem
		var folderPath, parentID string
		if err := rows.Scan(&item.ID, &item.Name, &parentID, &folderPath); err != nil {
			logger.L.Error("failed to scan recursive folder", zap.Error(err))
			continue
		}
		item.Type = "folder"
		item.Path = folderPath
		item.ParentID = &parentID
		*items = append(*items, item)
	}

	return rows.Err()
}

func (h *folderShareHandler) addRecursiveFiles(ctx context.Context, items *[]shareItem, folderID, userID string) error {
	rows, err := h.db.QueryContext(ctx, `
		WITH RECURSIVE folder_tree AS (
			-- Base case: start with the shared folder itself
			SELECT id, name, parent_id, name as folder_path
			FROM folders
			WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
			
			UNION ALL
			
			-- Recursive case: get children
			SELECT f.id, f.name, f.parent_id,
				   CASE 
					   WHEN ft.folder_path = '' THEN f.name
					   ELSE ft.folder_path || '/' || f.name
				   END as folder_path
			FROM folders f
			INNER JOIN folder_tree ft ON f.parent_id = ft.id
			WHERE f.user_id = $2 AND f.deleted_at IS NULL
		)
		SELECT uf.id, uf.filename, uf.original_size_bytes, uf.declared_mime, 
			   uf.folder_id, COALESCE(ft.folder_path, '') as folder_path
		FROM user_files uf
		JOIN folder_tree ft ON uf.folder_id = ft.id
		WHERE uf.user_id = $2 AND uf.deleted_at IS NULL
		ORDER BY folder_path, uf.filename
		LIMIT $3
	`, folderID, userID, maxFilesPerQuery)

	if err != nil {
		return fmt.Errorf("recursive file query failed: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var item shareItem
		var size int64
		var folderPath, parentFolderID string
		if err := rows.Scan(&item.ID, &item.Name, &size, &item.MimeType, &parentFolderID, &folderPath); err != nil {
			logger.L.Error("failed to scan recursive file", zap.Error(err))
			continue
		}
		item.Type = "file"
		item.Size = &size
		item.ParentID = &parentFolderID
		if folderPath == "" {
			item.Path = item.Name
		} else {
			item.Path = folderPath + "/" + item.Name
		}
		*items = append(*items, item)
	}

	return rows.Err()
}

func buildPath(pathParts []string) string {
	if len(pathParts) == 0 {
		return "/"
	}
	result := ""
	for _, part := range pathParts {
		result += "/" + part
	}
	return result
}

// downloadFromShareHandler generates presigned URL for file download
func downloadFromShareHandler(db *sql.DB, st *storage.MinioStorage, c cache.Cache) gin.HandlerFunc {
	producer := worker.NewProducer()

	return func(ctx *gin.Context) {
		token := ctx.Param("token")
		fileID := ctx.Param("fileId")

		// Validate share and get share info
		var shareID, targetFolderID string
		var recursive bool
		var expiresAt sql.NullTime

		err := db.QueryRowContext(ctx.Request.Context(), `
			SELECT id, target_id, recursive, expires_at
			FROM shares
			WHERE token=$1 AND target_type='folder' AND revoked=false
		`, token).Scan(&shareID, &targetFolderID, &recursive, &expiresAt)

		if err != nil {
			if err == sql.ErrNoRows {
				ctx.JSON(http.StatusNotFound, gin.H{"error": "share not found"})
				return
			}
			logger.L.Error("share lookup failed", zap.Error(err))
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			return
		}

		// Check expiry
		if expiresAt.Valid && expiresAt.Time.Before(time.Now()) {
			ctx.JSON(http.StatusGone, gin.H{"error": "share expired"})
			return
		}

		// Get file info
		var blobKey, filename string
		var fileFolderID sql.NullString
		var size int64

		err = db.QueryRowContext(ctx.Request.Context(), `
			SELECT uf.filename, fc.blob_key, fc.size_bytes, uf.folder_id
			FROM user_files uf
			JOIN file_contents fc ON uf.content_id = fc.id
			WHERE uf.id = $1 AND uf.deleted_at IS NULL
		`, fileID).Scan(&filename, &blobKey, &size, &fileFolderID)

		if err != nil {
			if err == sql.ErrNoRows {
				logger.L.Warn("file not found", zap.String("fileId", fileID))
				ctx.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
				return
			}
			logger.L.Error("file query failed", zap.Error(err))
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			return
		}

		// Check if file is accessible through this share
		accessible := false

		if !fileFolderID.Valid {
			logger.L.Warn("file has no folder", zap.String("fileId", fileID))
			ctx.JSON(http.StatusForbidden, gin.H{"error": "file not accessible"})
			return
		}

		// Check direct access (file is directly in the shared folder)
		if fileFolderID.String == targetFolderID {
			accessible = true
		} else if recursive {
			// Check if file's folder is in the shared tree (recursive access)
			var count int
			err = db.QueryRowContext(ctx.Request.Context(), `
				WITH RECURSIVE folder_tree AS (
					SELECT id FROM folders WHERE id = $1 AND deleted_at IS NULL
					UNION ALL
					SELECT f.id FROM folders f
					INNER JOIN folder_tree ft ON f.parent_id = ft.id
					WHERE f.deleted_at IS NULL
				)
				SELECT COUNT(*) FROM folder_tree WHERE id = $2
			`, targetFolderID, fileFolderID.String).Scan(&count)

			if err != nil {
				logger.L.Error("recursive folder check failed", zap.Error(err))
			} else if count > 0 {
				accessible = true
			}
		}

		if !accessible {
			logger.L.Warn("file not accessible through share",
				zap.String("fileId", fileID),
				zap.String("fileFolderId", fileFolderID.String),
				zap.String("targetFolderId", targetFolderID),
				zap.Bool("recursive", recursive))
			ctx.JSON(http.StatusForbidden, gin.H{"error": "file not accessible through this share"})
			return
		}

		// Generate presigned URL
		downloadURL, err := st.PresignedGetURL(ctx.Request.Context(), blobKey, 15)
		if err != nil {
			logger.L.Error("presigned URL generation failed", zap.Error(err))
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "download URL generation failed"})
			return
		}

		// Publish download event
		downloadEvent := worker.FolderShareEvent{
			Type:      "file_downloaded",
			ShareID:   shareID,
			FolderID:  targetFolderID,
			Token:     token,
			IPAddress: ctx.ClientIP(),
			Timestamp: time.Now().Format(time.RFC3339),
		}
		_ = producer.PublishFolderShareEvent(ctx.Request.Context(), downloadEvent)

		// Return download info
		ctx.JSON(http.StatusOK, gin.H{
			"downloadUrl": downloadURL,
			"filename":    filename,
			"size":        size,
			"expiresAt":   time.Now().Add(15 * time.Minute),
		})
	}
}

// downloadFolderArchiveHandler downloads the entire shared folder as a ZIP archive
func downloadFolderArchiveHandler(db *sql.DB, st *storage.MinioStorage, c cache.Cache) gin.HandlerFunc {
	producer := worker.NewProducer()

	return func(ctx *gin.Context) {
		token := ctx.Param("token")

		// Validate share and get share info
		var shareID, targetFolderID, folderName string
		var recursive bool
		var expiresAt sql.NullTime

		err := db.QueryRowContext(ctx.Request.Context(), `
			SELECT s.id, s.target_id, s.recursive, s.expires_at, f.name
			FROM shares s
			JOIN folders f ON s.target_id = f.id
			WHERE s.token=$1 AND s.target_type='folder' AND s.revoked=false
		`, token).Scan(&shareID, &targetFolderID, &recursive, &expiresAt, &folderName)

		if err != nil {
			if err == sql.ErrNoRows {
				ctx.JSON(http.StatusNotFound, gin.H{"error": "share not found"})
				return
			}
			logger.L.Error("share lookup failed", zap.Error(err))
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			return
		}

		// Check expiry
		if expiresAt.Valid && expiresAt.Time.Before(time.Now()) {
			ctx.JSON(http.StatusGone, gin.H{"error": "share expired"})
			return
		}

		// Get all files in the share (based on recursive setting)
		var items []shareItem

		// Get folder owner first
		var ownerID string
		err = db.QueryRowContext(ctx.Request.Context(), `
			SELECT user_id FROM folders WHERE id = $1 AND deleted_at IS NULL
		`, targetFolderID).Scan(&ownerID)
		if err != nil {
			logger.L.Error("failed to get folder owner for download", zap.Error(err))
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			return
		}

		if recursive {
			logger.L.Info("using recursive mode for folder share download",
				zap.String("token", token),
				zap.String("folderId", targetFolderID))
			items, _ = getRecursiveItemsStandalone(ctx.Request.Context(), db, targetFolderID, ownerID)
		} else {
			logger.L.Info("using direct mode for folder share download",
				zap.String("token", token),
				zap.String("folderId", targetFolderID))
			items, _ = getDirectItemsStandalone(ctx.Request.Context(), db, targetFolderID, ownerID)
		}

		logger.L.Info("found items in share",
			zap.String("token", token),
			zap.Int("totalItems", len(items)))

		// Filter only files (not folders)
		var files []shareItem
		for _, item := range items {
			if item.Type == "file" {
				files = append(files, item)
			}
		}

		logger.L.Info("filtered files for download",
			zap.String("token", token),
			zap.Int("fileCount", len(files)),
			zap.Bool("recursive", recursive))

		if len(files) == 0 {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "no files found in shared folder"})
			return
		}

		// Set headers for ZIP download
		zipFilename := fmt.Sprintf("%s.zip", folderName)
		ctx.Header("Content-Type", "application/zip")
		ctx.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", zipFilename))
		ctx.Header("Cache-Control", "no-cache")

		// Create ZIP writer
		zipWriter := zip.NewWriter(ctx.Writer)
		defer zipWriter.Close()

		// Add each file to the ZIP
		for _, file := range files {
			err := addFileToZip(ctx.Request.Context(), db, st, zipWriter, file)
			if err != nil {
				logger.L.Error("failed to add file to zip",
					zap.String("fileId", file.ID),
					zap.String("fileName", file.Name),
					zap.Error(err))
				// Continue with other files instead of failing entirely
				continue
			}
		}

		// Publish download event
		downloadEvent := worker.FolderShareEvent{
			Type:      "folder_downloaded",
			ShareID:   shareID,
			FolderID:  targetFolderID,
			Token:     token,
			IPAddress: ctx.ClientIP(),
			Timestamp: time.Now().Format(time.RFC3339),
		}
		_ = producer.PublishFolderShareEvent(ctx.Request.Context(), downloadEvent)
	}
}

// addFileToZip adds a single file to the ZIP archive
func addFileToZip(ctx context.Context, db *sql.DB, st *storage.MinioStorage, zipWriter *zip.Writer, file shareItem) error {
	// Get file's blob key
	var blobKey string
	err := db.QueryRowContext(ctx, `
		SELECT fc.blob_key
		FROM user_files uf
		JOIN file_contents fc ON uf.content_id = fc.id
		WHERE uf.id = $1 AND uf.deleted_at IS NULL
	`, file.ID).Scan(&blobKey)

	if err != nil {
		return fmt.Errorf("failed to get blob key: %w", err)
	}

	// Get presigned URL for the file
	downloadURL, err := st.PresignedGetURL(ctx, blobKey, 5) // 5 minute expiry for internal use
	if err != nil {
		return fmt.Errorf("failed to get presigned URL: %w", err)
	}

	// Download file content
	resp, err := http.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("failed to download file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("file download failed with status: %d", resp.StatusCode)
	}

	// Create file in ZIP
	zipFile, err := zipWriter.Create(file.Path)
	if err != nil {
		return fmt.Errorf("failed to create zip entry: %w", err)
	}

	// Copy file content to ZIP
	_, err = io.Copy(zipFile, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to copy file content: %w", err)
	}

	return nil
}

// Standalone helper functions for backward compatibility with non-handler functions
func getDirectItemsStandalone(ctx context.Context, db *sql.DB, folderID, userID string) ([]shareItem, int) {
	h := &folderShareHandler{db: db}
	items, err := h.getDirectItems(ctx, folderID, userID)
	if err != nil {
		logger.L.Error("getDirectItemsStandalone failed", zap.Error(err))
		return []shareItem{}, 0
	}
	return items, len(items)
}

func getRecursiveItemsStandalone(ctx context.Context, db *sql.DB, folderID, userID string) ([]shareItem, int) {
	h := &folderShareHandler{db: db}
	items, err := h.getRecursiveItems(ctx, folderID, userID)
	if err != nil {
		logger.L.Error("getRecursiveItemsStandalone failed", zap.Error(err))
		return []shareItem{}, 0
	}
	return items, len(items)
}
