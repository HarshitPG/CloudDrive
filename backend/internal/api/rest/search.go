package rest

import (
	"backend/internal/auth"
	"backend/internal/cache"
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

// Search Response Models
type searchFileItem struct {
	ID            string    `json:"id"`
	Filename      string    `json:"filename"`
	Mime          string    `json:"mime"`
	Size          int64     `json:"size"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
	DownloadCount int64     `json:"downloadCount"`
	ContentHash   string    `json:"contentHash"`
	PhysicalSize  int64     `json:"physicalSize"`
	RefCount      int64     `json:"refCount"`
	DedupSavings  int64     `json:"dedupSavings"`
	Rank          *float64  `json:"rank,omitempty"`
}

type searchFilesResponse struct {
	Items  []searchFileItem `json:"items"`
	Limit  int              `json:"limit"`
	Offset int              `json:"offset"`
	Total  int64            `json:"total"`
}

func RegisterSearchRoutes(rg *gin.RouterGroup, db *sql.DB, jwtSecret string, c cache.Cache) {
	s := rg.Group("/search")
	s.Use(auth.RequireAuth(jwtSecret))
	h := &searchHandler{db: db}
	s.GET("/files", h.searchFiles)
}

type searchHandler struct {
	db    *sql.DB
	cache cache.Cache
}

// SearchFiles godoc
//
//	@Summary		Search files across the system
//	@Description	Search for files using various filters including query text, MIME type, folder location, and uploader
//	@Tags			search
//	@Produce		json
//	@Param			q			query		string				false	"Search query text for file names and content"
//	@Param			mime		query		string				false	"Filter by MIME type (e.g., image/jpeg, text/plain)"
//	@Param			folderId	query		string				false	"Filter by parent folder ID"
//	@Param			uploader	query		string				false	"Filter by uploader user ID"
//	@Param			sort		query		string				false	"Sort order (name, size, created_at, updated_at)"
//	@Param			limit		query		int					false	"Maximum results to return (1-200, default: 50)"
//	@Param			offset		query		int					false	"Number of results to skip for pagination (default: 0)"
//	@Success		200			{object}	searchFilesResponse	"Search results with files array and metadata"
//	@Failure		401			{object}	errorResponse		"Unauthorized"
//	@Failure		500			{object}	errorResponse		"Internal server error"
//	@Security		BearerAuth
//	@Router			/api/v1/search/files [get]
func (h *searchHandler) searchFiles(c *gin.Context) {
	userID := auth.GetUserIDFromCtx(c.Request.Context())
	if userID == "" {
		c.JSON(http.StatusUnauthorized, errorResponse{Error: "unauthenticated"})
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

	// Try cache
	cacheKey := cache.SearchKey(userID, map[string]string{
		"q":        p.Q,
		"mime":     p.Mime,
		"folderId": p.FolderID,
		"sort":     p.Sort,
		"limit":    strconv.Itoa(limit),
		"offset":   strconv.Itoa(offset),
	})
	if h.cache != nil {
		var cached searchFilesResponse
		if err := h.cache.Get(ctx, cacheKey, &cached); err == nil {
			c.JSON(http.StatusOK, cached)
			return
		}
	}

	query, countQuery, args := search.BuildQuery(p)
	logger.L.Debug("search query built",
		zap.String("query", query),
		zap.Any("args", args),
		zap.String("userID", userID),
	)

	rows, err := h.db.QueryContext(ctx, query, args...)
	if err != nil {
		logger.L.Error("search rows query failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "internal"})
		return
	}
	defer rows.Close()

	var total int64
	if err := h.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		logger.L.Warn("search count query failed", zap.Error(err))
		total = -1
	}

	results := []searchFileItem{}
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
			c.JSON(http.StatusInternalServerError, errorResponse{Error: "internal scan error"})
			return
		}

		item := searchFileItem{
			ID:            id,
			Filename:      filename,
			Mime:          mime,
			Size:          size,
			CreatedAt:     createdAt,
			UpdatedAt:     updatedAt,
			DownloadCount: downloadCount,
			ContentHash:   contentHash,
			PhysicalSize:  contentSize,
			RefCount:      refCount,
			DedupSavings:  size - contentSize,
			Rank:          rank,
		}
		results = append(results, item)
	}

	resp := searchFilesResponse{
		Items:  results,
		Limit:  limit,
		Offset: offset,
		Total:  total,
	}
	// Write-through cache
	if h.cache != nil {
		_ = h.cache.Set(ctx, cacheKey, resp, 30*time.Second)
	}
	c.JSON(http.StatusOK, resp)
}
