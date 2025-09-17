package rest

import (
	"backend/internal/auth"
	"backend/internal/search"
	"backend/pkg/logger"
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func RegisterSearchRoutes(rg *gin.RouterGroup, db *sql.DB, jwtSecret string) {
	s := rg.Group("/search")
	s.Use(auth.RequireAuth(jwtSecret))
	h := &searchHandler{db: db}
	s.GET("/files", h.searchFiles)
}

type searchHandler struct {
	db *sql.DB
}

func (h *searchHandler) searchFiles(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}

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

	p := search.Params{
		UserID:        userID,
		Q:             c.Query("q"),
		Mime:          c.Query("mime"),
		FolderID:      c.Query("folderId"),
		Uploader:      c.Query("uploader"),
		Sort:          c.DefaultQuery("sort", "created_at_desc"),
		Limit:         limit,
		Offset:        offset,
		IncludeRank:   c.Query("q") != "",
		IncludeShared: true,
	}

	if v := c.Query("minSize"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			p.MinSize = &n
		}
	}
	if v := c.Query("maxSize"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			p.MaxSize = &n
		}
	}
	if v := c.Query("dateFrom"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			p.DateFrom = &t
		}
	}
	if v := c.Query("dateTo"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			p.DateTo = &t
		}
	}
	if v := c.Query("tags"); v != "" {
		parts := strings.Split(v, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		p.Tags = parts
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	query, countQuery, args := search.BuildQuery(p)
	logger.L.Debug("search query built",
		zap.String("query", query),
		zap.Any("args", args),
		zap.String("userID", userID),
	)

	rows, err := h.db.QueryContext(ctx, query, args...)
	if err != nil {
		logger.L.Error("search rows query failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	defer rows.Close()

	var total int64
	if err := h.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		logger.L.Warn("search count query failed", zap.Error(err))
		total = -1
	}

	results := []map[string]interface{}{}
	for rows.Next() {
		var (
			id, filename, mime, contentHash string
			size, contentSize, refCount     int64
			downloadCount                   int64
			createdAt, updatedAt            time.Time
			rank                            *float64
		)

		if scanErr := rows.Scan(
			&id, &filename, &mime, &size,
			&createdAt, &updatedAt, &downloadCount,
			&contentHash, &contentSize, &refCount, &rank,
		); scanErr != nil {
			logger.L.Error("row scan failed", zap.Error(scanErr))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal scan error"})
			return
		}

		item := map[string]interface{}{
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
		}
		if rank != nil {
			item["rank"] = *rank
		}
		results = append(results, item)
	}

	resp := gin.H{
		"items":  results,
		"limit":  limit,
		"offset": offset,
		"total":  total,
	}
	c.JSON(http.StatusOK, resp)
}
