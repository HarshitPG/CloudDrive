package rest

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"

	"backend/internal/audit"
	"backend/internal/auth"
	"backend/internal/cache"
	"backend/internal/storage"
	"backend/internal/utils"
	"backend/internal/worker"
	"backend/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"go.uber.org/zap"
)

func RegisterUploadRoutes(rg *gin.RouterGroup, db *sql.DB, st *storage.MinioStorage, jwtSecret string) {
	uploads := rg.Group("/uploads")
	uploads.Use(auth.RequireAuth(jwtSecret))
	h := &uploadHandler{
		db:       db,
		storage:  st,
		producer: worker.NewProducer(),
	}

	uploads.POST("/session", h.createSession)
	uploads.POST("/complete", h.complete)
	uploads.POST("/abort", h.abort)
	uploads.POST("/folder/init", h.folderInit)
}

type uploadHandler struct {
	db       *sql.DB
	storage  *storage.MinioStorage
	cache    cache.Cache
	producer *worker.Producer
}

type FolderInitFile struct {
	Path   string `json:"path" binding:"required"`
	Size   int64  `json:"size" binding:"required"`
	Mime   string `json:"mime"`
	SHA256 string `json:"sha256"`
}

type FolderInitRequest struct {
	ParentID       string           `json:"parentId"`
	RootName       string           `json:"rootName" binding:"required"`
	Files          []FolderInitFile `json:"files" binding:"required"`
	IdempotencyKey string           `json:"idempotencyKey"`
}

type FolderInitFileResponse struct {
	Path           string `json:"path"`
	Deduped        bool   `json:"deduped"`
	UserFileID     string `json:"userFileId,omitempty"`
	SessionID      string `json:"sessionId,omitempty"`
	UploadUrl      string `json:"uploadUrl,omitempty"`
	TempBlobKey    string `json:"tempBlobKey,omitempty"`
	TargetFolderID string `json:"targetFolderId,omitempty"`
}

type FolderInitResponse struct {
	UploadID     string                   `json:"uploadId"`
	RootFolderID string                   `json:"rootFolderId"`
	Folders      []map[string]string      `json:"folders"`
	Files        []FolderInitFileResponse `json:"files"`
}

// folderInit builds the folder tree under parent and prepares uploads or dedup fast-paths per file.
func (h *uploadHandler) folderInit(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}

	var req FolderInitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.Files) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no files provided"})
		return
	}

	tx, err := h.db.BeginTx(c.Request.Context(), nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	defer tx.Rollback()

	// Create or reuse root folder
	var rootFolderID string
	q := `SELECT id FROM folders WHERE user_id=$1 AND COALESCE(parent_id::text,'') = NULLIF($2,'') AND name=$3 AND deleted_at IS NULL LIMIT 1`
	err = tx.QueryRowContext(c.Request.Context(), q, userID, req.ParentID, req.RootName).Scan(&rootFolderID)
	if err == sql.ErrNoRows {
		err = tx.QueryRowContext(c.Request.Context(), `
			INSERT INTO folders (id, user_id, parent_id, name, created_at, updated_at)
			VALUES (gen_random_uuid(), $1, NULLIF($2,'')::uuid, $3, now(), now())
			RETURNING id
		`, userID, req.ParentID, req.RootName).Scan(&rootFolderID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
			return
		}
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}

	// Ensure subfolders exist
	// map of path -> folderID; root path is ""
	folderMap := map[string]string{"": rootFolderID}

	ensureFolder := func(path string) (string, error) {
		if id, ok := folderMap[path]; ok {
			return id, nil
		}
		// split path into segments
		// find parent path id progressively
		for i, ch := range path {
			if ch == '/' {
				seg := path[:i]
				if seg != "" {
					if _, ok := folderMap[seg]; !ok {
						// recursively ensure parent of seg
						lastSlash := -1
						for j := len(seg) - 1; j >= 0; j-- {
							if seg[j] == '/' {
								lastSlash = j
								break
							}
						}
						var pp string
						var name string
						if lastSlash >= 0 {
							pp = seg[:lastSlash]
							name = seg[lastSlash+1:]
						} else {
							pp = ""
							name = seg
						}
						pid := folderMap[pp]
						// create or reuse folder
						var fid string
						if err := tx.QueryRowContext(c.Request.Context(),
							`SELECT id FROM folders WHERE user_id=$1 AND parent_id=$2 AND name=$3 AND deleted_at IS NULL LIMIT 1`,
							userID, pid, name,
						).Scan(&fid); err == sql.ErrNoRows {
							if err := tx.QueryRowContext(c.Request.Context(),
								`INSERT INTO folders (id, user_id, parent_id, name, created_at, updated_at)
								 VALUES (gen_random_uuid(), $1, $2, $3, now(), now()) RETURNING id`,
								userID, pid, name,
							).Scan(&fid); err != nil {
								return "", err
							}
						} else if err != nil {
							return "", err
						}
						folderMap[seg] = fid
					}
				}
			}
		}
		// Create final folder if not yet present
		// path might be a single segment without slash
		if path != "" {
			if _, ok := folderMap[path]; !ok {
				lastSlash := -1
				for j := len(path) - 1; j >= 0; j-- {
					if path[j] == '/' {
						lastSlash = j
						break
					}
				}
				var pp string
				var name string
				if lastSlash >= 0 {
					pp = path[:lastSlash]
					name = path[lastSlash+1:]
				} else {
					pp = ""
					name = path
				}
				pid := folderMap[pp]
				var fid string
				if err := tx.QueryRowContext(c.Request.Context(),
					`SELECT id FROM folders WHERE user_id=$1 AND parent_id=$2 AND name=$3 AND deleted_at IS NULL LIMIT 1`,
					userID, pid, name,
				).Scan(&fid); err == sql.ErrNoRows {
					if err := tx.QueryRowContext(c.Request.Context(),
						`INSERT INTO folders (id, user_id, parent_id, name, created_at, updated_at)
						 VALUES (gen_random_uuid(), $1, $2, $3, now(), now()) RETURNING id`,
						userID, pid, name,
					).Scan(&fid); err != nil {
						return "", err
					}
				} else if err != nil {
					return "", err
				}
				folderMap[path] = fid
			}
		}
		return folderMap[path], nil
	}

	// Build set of directories from file paths
	dirSet := map[string]struct{}{}
	for _, f := range req.Files {
		p := f.Path
		// directory is everything before last '/'
		last := -1
		for i := len(p) - 1; i >= 0; i-- {
			if p[i] == '/' {
				last = i
				break
			}
		}
		dir := ""
		if last > 0 {
			dir = p[:last]
		}
		if dir != "" {
			dirSet[dir] = struct{}{}
		}
	}
	// Ensure each directory exists
	for d := range dirSet {
		if _, err := ensureFolder(d); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
			return
		}
	}

	// Prepare responses
	responses := make([]FolderInitFileResponse, 0, len(req.Files))
	foldersOut := []map[string]string{}
	for path, id := range folderMap {
		foldersOut = append(foldersOut, map[string]string{"path": path, "folderId": id})
	}

	// For files: dedup fast path or create sessions
	for _, f := range req.Files {
		// target directory id
		// compute dir
		last := -1
		for i := len(f.Path) - 1; i >= 0; i-- {
			if f.Path[i] == '/' {
				last = i
				break
			}
		}
		dir := ""
		name := f.Path
		if last >= 0 {
			dir = f.Path[:last]
			name = f.Path[last+1:]
		}
		targetFID := rootFolderID
		if dir != "" {
			if id, ok := folderMap[dir]; ok {
				targetFID = id
			}
		}

		// If SHA256 provided and matches existing content, create user_files immediately
		if f.SHA256 != "" {
			var contentID string
			var sizeBytes int64
			err := tx.QueryRowContext(c.Request.Context(),
				"SELECT id, size_bytes FROM file_contents WHERE content_hash=$1 LIMIT 1",
				f.SHA256,
			).Scan(&contentID, &sizeBytes)
			if err == nil {
				var newUserFileID string
				err = tx.QueryRowContext(c.Request.Context(), `
					INSERT INTO user_files (id, user_id, content_id, filename, declared_mime, original_size_bytes, folder_id, created_at, updated_at)
					VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, now(), now())
					RETURNING id
				`, userID, contentID, name, f.Mime, f.Size, targetFID).Scan(&newUserFileID)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
					return
				}
				if _, err := tx.ExecContext(c.Request.Context(),
					"UPDATE file_contents SET ref_count = ref_count + 1 WHERE id=$1", contentID); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
					return
				}
				responses = append(responses, FolderInitFileResponse{
					Path: f.Path, Deduped: true, UserFileID: newUserFileID,
				})
				continue
			}
		}

		// Otherwise, create an upload session and presigned URL
		sessionID := uuid.NewString()
		tempName := fmt.Sprintf("tmp/%s/%s", sessionID, name)
		url, err := h.storage.PresignedPutURL(c.Request.Context(), tempName, 30)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed generate upload url"})
			return
		}
		if _, err := tx.ExecContext(c.Request.Context(), `
			INSERT INTO upload_sessions (id, user_id, filename, declared_mime, original_size_bytes, temp_blob_key, client_sha256, status, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,'OPEN',now(),now())
		`, sessionID, userID, name, f.Mime, f.Size, tempName, f.SHA256); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed create session"})
			return
		}
		responses = append(responses, FolderInitFileResponse{
			Path: f.Path, Deduped: false, SessionID: sessionID, UploadUrl: url, TempBlobKey: tempName, TargetFolderID: targetFID,
		})
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}

	c.JSON(http.StatusOK, FolderInitResponse{
		UploadID:     uuid.NewString(),
		RootFolderID: rootFolderID,
		Folders:      foldersOut,
		Files:        responses,
	})
}

type createSessionRequest struct {
	Filename     string `json:"filename" binding:"required"`
	DeclaredMime string `json:"declaredMime"`
	OriginalSize int64  `json:"originalSize" binding:"required"`
	ClientSha256 string `json:"clientSha256"`
}

type createSessionResponse struct {
	SessionId      string `json:"sessionId,omitempty"`
	UploadUrl      string `json:"uploadUrl,omitempty"`
	TempBlobKey    string `json:"tempBlobKey,omitempty"`
	SkipUpload     bool   `json:"skipUpload"`
	ExistingFileId string `json:"existingFileId,omitempty"`
	UserFileId     string `json:"userFileId,omitempty"`
}

func (h *uploadHandler) createSession(c *gin.Context) {
	var req createSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}

	if req.OriginalSize > 0 {
		used, quota, err := h.getUsageAndQuota(c.Request.Context(), userID)
		if err != nil {
			logger.L.Error("quota lookup failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
			return
		}
		if used+req.OriginalSize > quota {
			c.JSON(http.StatusForbidden, gin.H{
				"error":      "quota exceeded",
				"usedBytes":  used,
				"quotaBytes": quota,
				"attempt":    req.OriginalSize,
			})
			return
		}
	}

	if req.ClientSha256 != "" {
		var contentID string
		var sizeBytes int64
		err := h.db.QueryRowContext(c.Request.Context(),
			"SELECT id, size_bytes FROM file_contents WHERE content_hash=$1 LIMIT 1",
			req.ClientSha256).Scan(&contentID, &sizeBytes)

		if err == nil {
			var existingUserFile string
			err = h.db.QueryRowContext(c.Request.Context(),
				"SELECT id FROM user_files WHERE user_id=$1 AND content_id=$2 AND deleted_at IS NULL LIMIT 1",
				userID, contentID).Scan(&existingUserFile)
			if err == nil {
				c.JSON(http.StatusOK, createSessionResponse{
					SkipUpload:     true,
					ExistingFileId: existingUserFile,
					UserFileId:     existingUserFile,
				})
				return
			}

			tx, txErr := h.db.BeginTx(c.Request.Context(), nil)
			if txErr != nil {
				logger.L.Error("fastpath tx begin failed", zap.Error(txErr))
				c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
				return
			}
			defer tx.Rollback()

			var newUserFileID string
			err = tx.QueryRowContext(c.Request.Context(), `
				INSERT INTO user_files (id, user_id, content_id, filename, declared_mime, original_size_bytes, created_at, updated_at)
				VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, now(), now())
				RETURNING id
			`, userID, contentID, req.Filename, req.DeclaredMime, req.OriginalSize).Scan(&newUserFileID)
			if err != nil {
				logger.L.Error("fastpath insert user_files failed", zap.Error(err))
				c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
				return
			}
			if _, err := tx.ExecContext(c.Request.Context(),
				"UPDATE file_contents SET ref_count = ref_count + 1 WHERE id=$1", contentID); err != nil {
				logger.L.Error("fastpath refcount inc failed", zap.Error(err))
				c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
				return
			}
			if err := tx.Commit(); err != nil {
				logger.L.Error("fastpath tx commit failed", zap.Error(err))
				c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
				return
			}

			c.JSON(http.StatusOK, createSessionResponse{
				SkipUpload:     true,
				ExistingFileId: newUserFileID,
				UserFileId:     newUserFileID,
			})
			return
		}
	}

	sessionID := uuid.NewString()
	tempName := fmt.Sprintf("tmp/%s/%s", sessionID, req.Filename)
	url, err := h.storage.PresignedPutURL(c.Request.Context(), tempName, 30)
	if err != nil {
		logger.L.Error("presigned url fail", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed generate upload url"})
		return
	}
	_, err = h.db.ExecContext(c.Request.Context(), `
		INSERT INTO upload_sessions (id, user_id, filename, declared_mime, original_size_bytes, temp_blob_key, client_sha256, status, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,'OPEN',now(),now())
	`, sessionID, userID, req.Filename, req.DeclaredMime, req.OriginalSize, tempName, req.ClientSha256)
	if err != nil {
		logger.L.Error("insert upload session failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed create session"})
		return
	}
	c.JSON(http.StatusCreated, createSessionResponse{
		SessionId:   sessionID,
		UploadUrl:   url,
		TempBlobKey: tempName,
		SkipUpload:  false,
	})
}

type completeRequest struct {
	SessionId    string `json:"sessionId" binding:"required"`
	ClientSha256 string `json:"clientSha256"`
	FolderID     string `json:"folderId"`
}

func (h *uploadHandler) complete(c *gin.Context) {
	var req completeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}

	var tempKey, filename, declaredMime, clientSha string
	var originalSize int64
	var status string
	err := h.db.QueryRowContext(c.Request.Context(),
		"SELECT temp_blob_key, filename, declared_mime, original_size_bytes, status, client_sha256 FROM upload_sessions WHERE id=$1",
		req.SessionId).Scan(&tempKey, &filename, &declaredMime, &originalSize, &status, &clientSha)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
			return
		}
		logger.L.Error("db err", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}
	if status != "OPEN" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session not open"})
		return
	}

	sha := req.ClientSha256
	if sha == "" && clientSha != "" {
		sha = clientSha
	}

	objReader, err := h.storage.GetObjectReader(c.Request.Context(), tempKey)
	if err != nil {
		logger.L.Error("get temp object failed", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "uploaded object not found"})
		return
	}
	head := make([]byte, 512)
	n, _ := io.ReadFull(objReader, head)

	if sha == "" {
		contentReader := io.MultiReader(bytes.NewReader(head[:n]), objReader)
		computed, err := utils.ComputeSHA256(contentReader)
		if err != nil {
			_ = objReader.Close()
			logger.L.Error("hash compute failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "hash compute failed"})
			return
		}
		sha = computed
	}
	_ = objReader.Close()

	sniffedMime := http.DetectContentType(head[:n])
	if declaredMime != "" && declaredMime != sniffedMime {
		ext := filepath.Ext(filename)
		alias := mime.TypeByExtension(ext)
		if !(alias == sniffedMime || alias == declaredMime) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":        "declared mime mismatch",
				"declaredMime": declaredMime,
				"sniffedMime":  sniffedMime,
			})
			return
		}
	}
	if declaredMime == "" {
		declaredMime = sniffedMime
	}

	tx, err := h.db.BeginTx(c.Request.Context(), nil)
	if err != nil {
		logger.L.Error("tx begin failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	defer tx.Rollback()

	var contentID string
	err = tx.QueryRowContext(c.Request.Context(),
		"SELECT id FROM file_contents WHERE content_hash=$1 LIMIT 1",
		sha).Scan(&contentID)

	if err == nil {
		var newUserFileID string
		err = tx.QueryRowContext(c.Request.Context(), `
			INSERT INTO user_files (id, user_id, content_id, filename, declared_mime, original_size_bytes, folder_id, created_at, updated_at)
			VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, NULLIF($6,'')::uuid, now(), now())
			RETURNING id
		`, userID, contentID, filename, declaredMime, originalSize, req.FolderID).Scan(&newUserFileID)
		if err != nil {
			logger.L.Error("insert user_files dedup failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
			return
		}
		if _, err := tx.ExecContext(c.Request.Context(),
			"UPDATE file_contents SET ref_count = ref_count + 1 WHERE id=$1", contentID); err != nil {
			logger.L.Error("refcount inc failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
			return
		}
		_, _ = tx.ExecContext(c.Request.Context(), "UPDATE upload_sessions SET status='COMPLETED', client_sha256=$2, updated_at=now() WHERE id=$1", req.SessionId, sha)
		if err := tx.Commit(); err != nil {
			logger.L.Error("commit failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
			return
		}

		// Invalidate cache
		if h.cache != nil && req.FolderID != "" {
			cache.InvalidateFolder(c.Request.Context(), h.cache, req.FolderID)
		}

		c.JSON(http.StatusOK, gin.H{"userFileId": newUserFileID, "deduped": true})
		return
	}

	used, quota, errQ := h.getUsageAndQuota(c.Request.Context(), userID)
	if errQ != nil {
		logger.L.Error("quota lookup failed", zap.Error(errQ))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	if used+originalSize > quota {
		c.JSON(http.StatusForbidden, gin.H{
			"error":      "quota exceeded",
			"usedBytes":  used,
			"quotaBytes": quota,
			"attempt":    originalSize,
		})
		return
	}

	finalKey := fmt.Sprintf("objects/%s", sha)
	if err := h.storage.CopyTempToObject(c.Request.Context(), tempKey, finalKey); err != nil {
		logger.L.Error("move temp to final failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "storage move failed"})
		return
	}

	var newContentID string
	err = tx.QueryRowContext(c.Request.Context(), `
		INSERT INTO file_contents (id, content_hash, blob_key, size_bytes, mime_type, ref_count, created_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, 1, now())
		RETURNING id
	`, sha, finalKey, originalSize, declaredMime).Scan(&newContentID)
	if err != nil {
		logger.L.Warn("insert file_contents failed, fallback to select", zap.Error(err))
		err2 := tx.QueryRowContext(c.Request.Context(),
			"SELECT id FROM file_contents WHERE content_hash=$1 LIMIT 1", sha).Scan(&newContentID)
		if err2 != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
			return
		}
		if _, err := tx.ExecContext(c.Request.Context(),
			"UPDATE file_contents SET ref_count = ref_count + 1 WHERE id=$1", newContentID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
			return
		}
	}

	var newUserFileID string
	err = tx.QueryRowContext(c.Request.Context(), `
		INSERT INTO user_files (id, user_id, content_id, filename, declared_mime, original_size_bytes, folder_id, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, NULLIF($6,'')::uuid, now(), now())
		RETURNING id
	`, userID, newContentID, filename, declaredMime, originalSize, req.FolderID).Scan(&newUserFileID)
	if err != nil {
		logger.L.Error("insert user_files final failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}

	_, _ = tx.ExecContext(c.Request.Context(), "UPDATE upload_sessions SET status='COMPLETED', client_sha256=$2, updated_at=now() WHERE id=$1", req.SessionId, sha)
	_ = audit.Log(
		c.Request.Context(),
		h.db,
		userID,
		"upload",
		"file",
		newUserFileID,
		map[string]interface{}{
			"filename": filename,
			"size":     originalSize,
			"sha256":   sha,
		},
	)

	if err := tx.Commit(); err != nil {
		logger.L.Error("tx commit failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}

	// Invalidate cache
	if h.cache != nil && req.FolderID != "" {
		cache.InvalidateFolder(c.Request.Context(), h.cache, req.FolderID)
	}

	c.JSON(http.StatusOK, gin.H{"userFileId": newUserFileID, "contentId": newContentID, "deduped": false})
}

type abortRequest struct {
	SessionId string `json:"sessionId" binding:"required"`
}

func (h *uploadHandler) abort(c *gin.Context) {
	var req abortRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	var tempKey string
	err := h.db.QueryRowContext(c.Request.Context(),
		"SELECT temp_blob_key FROM upload_sessions WHERE id=$1 AND user_id=$2", req.SessionId, userID).Scan(&tempKey)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}
	err = h.storage.Client.RemoveObject(c.Request.Context(), h.storage.Bucket, tempKey, minio.RemoveObjectOptions{})
	if err != nil {
		logger.L.Warn("remove temp object failed", zap.Error(err))
	}
	_, _ = h.db.ExecContext(c.Request.Context(), "UPDATE upload_sessions SET status='ABORTED', updated_at=now() WHERE id=$1", req.SessionId)
	c.JSON(http.StatusOK, gin.H{"message": "aborted"})
}

func (h *uploadHandler) getUsageAndQuota(ctx context.Context, userID string) (int64, int64, error) {
	var used int64
	if err := h.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(original_size_bytes),0)
		FROM user_files
		WHERE user_id=$1 AND deleted_at IS NULL
	`, userID).Scan(&used); err != nil {
		return 0, 0, err
	}
	var quota int64
	if err := h.db.QueryRowContext(ctx,
		"SELECT quota_bytes FROM users WHERE id=$1", userID).Scan(&quota); err != nil {
		return 0, 0, err
	}
	return used, quota, nil
}
