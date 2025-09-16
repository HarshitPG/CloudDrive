package rest

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"

	"backend/internal/auth"
	"backend/internal/search"
	"backend/pkg/logger"

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
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 1000 {
			limit = n
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
		IncludeRank:   true,
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

	query, countQuery, args := search.BuildQuery(p)

	type rowsResult struct {
		rows *sql.Rows
		err  error
	}
	rowsCh := make(chan rowsResult, 1)
	countCh := make(chan struct {
		n   int64
		err error
	}, 1)

	go func() {
		rows, err := h.db.QueryContext(c.Request.Context(), query, args...)
		rowsCh <- rowsResult{rows: rows, err: err}
	}()

	go func() {
		var total int64
		err := h.db.QueryRowContext(c.Request.Context(), countQuery, args...).Scan(&total)
		countCh <- struct {
			n   int64
			err error
		}{n: total, err: err}
	}()

	rowsRes := <-rowsCh
	if rowsRes.err != nil {
		logger.L.Error("search rows failed", zap.Error(rowsRes.err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	defer rowsRes.rows.Close()

	countRes := <-countCh
	if countRes.err != nil {
		logger.L.Warn("search count failed", zap.Error(countRes.err))
	}

	results := []map[string]interface{}{}
	for rowsRes.rows.Next() {
		var id, filename, mime, contentHash string
		var size, contentSize, refCount, downloadCount int64
		var createdAt, updatedAt time.Time
		var rank *float64

		scanErr := rowsRes.rows.Scan(
			&id, &filename, &mime, &size,
			&createdAt, &updatedAt, &downloadCount,
			&contentHash, &contentSize, &refCount, &rank,
		)
		if scanErr != nil {
			_ = rowsRes.rows.Scan(
				&id, &filename, &mime, &size,
				&createdAt, &updatedAt, &downloadCount,
				&contentHash, &contentSize, &refCount,
			)
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
		"total":  countRes.n,
	}
	c.JSON(http.StatusOK, resp)
}
