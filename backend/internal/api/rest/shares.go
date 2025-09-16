package rest

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/http"
	"time"

	"backend/internal/auth"
	"backend/internal/storage"
	"backend/pkg/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func RegisterShareRoutes(rg *gin.RouterGroup, db *sql.DB, st *storage.MinioStorage, jwtSecret string) {
	rg.GET("/s/:token", resolveShareHandler(db, st))

	protected := rg.Group("/shares")
	protected.Use(auth.RequireAuth(jwtSecret))
	h := &shareHandler{db: db, storage: st}

	protected.POST("/files/:id/share", h.createPublicFileShare)
	protected.DELETE("/shares/:id", h.revokeShare)
	protected.POST("/files/:id/share/user", h.shareFileToUser)
	protected.GET("/files/:id/shares", h.listFileShares)
	protected.POST("/folders/:id/share", h.createPublicFolderShare)
}

type shareHandler struct {
	db      *sql.DB
	storage *storage.MinioStorage
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
	TargetUserId string `json:"targetUserId" binding:"required"`
	Permission   string `json:"permission"`
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
	`, shareID, req.TargetUserId, req.Permission)
	if err != nil {
		logger.L.Error("insert share_user failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}

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
	c.JSON(http.StatusOK, gin.H{"message": "revoked"})
}

// --- PUBLIC RESOLVE ---
// GET /s/:token
// No auth required. If token points to file -> return metadata + presigned download url(s).
// If token points to folder -> return folder listing + presigned urls for files (first N).
func resolveShareHandler(db *sql.DB, st *storage.MinioStorage) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.Param("token")

		var id, targetType, targetId string
		var expires sql.NullTime
		err := db.QueryRowContext(c.Request.Context(), `
			SELECT id, target_type, target_id, expires_at
			FROM shares
			WHERE token=$1 AND revoked=false
		`, token).Scan(&id, &targetType, &targetId, &expires)
		if err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusNotFound, gin.H{"error": "share not found"})
				return
			}
			logger.L.Error("db error", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
			return
		}
		if expires.Valid && expires.Time.Before(time.Now()) {
			c.JSON(http.StatusGone, gin.H{"error": "share expired"})
			return
		}

		ctx := c.Request.Context()
		switch targetType {
		case "file":
			var ufid, filename, blobKey string
			var size int64
			err := db.QueryRowContext(ctx, `
				SELECT uf.id, uf.filename, fc.blob_key, fc.size_bytes
				FROM user_files uf
				JOIN file_contents fc ON uf.content_id = fc.id
				WHERE uf.id=$1 AND uf.deleted_at IS NULL
			`, targetId).Scan(&ufid, &filename, &blobKey, &size)
			if err != nil {
				if err == sql.ErrNoRows {
					c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
					return
				}
				logger.L.Error("db error", zap.Error(err))
				c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
				return
			}

			url, err := st.PresignedGetURL(ctx, blobKey, 15)
			if err != nil {
				logger.L.Error("presign failed", zap.Error(err))
				c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
				return
			}

			_, _ = db.ExecContext(ctx, "UPDATE user_files SET download_count = download_count + 1 WHERE id=$1", ufid)
			_, _ = db.ExecContext(ctx, "INSERT INTO audit_logs (user_id, action, target_type, target_id, created_at) VALUES (NULL,'public_download','file',$1,now())", ufid)

			c.JSON(http.StatusOK, gin.H{
				"type":     "file",
				"fileId":   ufid,
				"filename": filename,
				"size":     size,
				"download": url,
			})
			return

		case "folder":
			foldersRows, _ := db.QueryContext(ctx, `
				SELECT id, name
				FROM folders
				WHERE id = $1 AND deleted_at IS NULL
			`, targetId)
			defer func() {
				if foldersRows != nil {
					_ = foldersRows.Close()
				}
			}()

			rows, err := db.QueryContext(ctx, `
				SELECT uf.id, uf.filename, fc.blob_key, fc.size_bytes
				FROM user_files uf
				JOIN file_contents fc ON uf.content_id = fc.id
				WHERE uf.folder_id = $1 AND uf.deleted_at IS NULL
				LIMIT 100
			`, targetId)
			if err != nil {
				logger.L.Error("db err", zap.Error(err))
				c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
				return
			}
			defer rows.Close()

			files := []map[string]interface{}{}
			for rows.Next() {
				var fid, fname, blob string
				var fsize int64
				_ = rows.Scan(&fid, &fname, &blob, &fsize)
				url, _ := st.PresignedGetURL(ctx, blob, 15)
				files = append(files, map[string]interface{}{
					"id":       fid,
					"filename": fname,
					"size":     fsize,
					"download": url,
				})
			}
			c.JSON(http.StatusOK, gin.H{
				"type":  "folder",
				"files": files,
			})
			return

		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "unknown share target"})
			return
		}
	}
}
