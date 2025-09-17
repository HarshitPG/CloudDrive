package graphql

import (
	"backend/internal/api/graphql/generated"
	"backend/internal/api/graphql/model"
	"backend/internal/auth"
	"backend/internal/search"
	"backend/pkg/logger"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"
)

type Resolver struct{ DB *sql.DB }

func (r *queryResolver) SearchFiles(
	ctx context.Context,
	q *string,
	mime *string,
	minSize *int,
	maxSize *int,
	dateFrom *string,
	dateTo *string,
	tags []string,
	uploader *string,
	folderID *string,
	limit *int,
	offset *int,
	sort *string,
) (*model.FileSearchResponse, error) {
	userID := auth.GetUserIDFromCtx(ctx)
	if userID == "" {
		return nil, fmt.Errorf("unauthenticated")
	}

	lim := defaultInt(limit, 50)
	if lim > 200 {
		lim = 200
	}
	off := defaultInt(offset, 0)
	if off < 0 {
		off = 0
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	qVal := deref(q)
	p := search.Params{
		UserID:        userID,
		Q:             qVal,
		Mime:          deref(mime),
		FolderID:      deref(folderID),
		Uploader:      deref(uploader),
		Sort:          defaultStr(sort, "created_at_desc"),
		Limit:         lim,
		Offset:        off,
		IncludeRank:   qVal != "",
		IncludeShared: true,
	}

	if minSize != nil {
		v := int64(*minSize)
		p.MinSize = &v
	}
	if maxSize != nil {
		v := int64(*maxSize)
		p.MaxSize = &v
	}
	if dateFrom != nil {
		if t, err := time.Parse(time.RFC3339, *dateFrom); err == nil {
			p.DateFrom = &t
		}
	}
	if dateTo != nil {
		if t, err := time.Parse(time.RFC3339, *dateTo); err == nil {
			p.DateTo = &t
		}
	}
	if len(tags) > 0 {
		p.Tags = tags
	}

	query, countQuery, args := search.BuildQuery(p)
	logger.L.Debug("search query built",
		zap.String("query", query),
		zap.Any("args", args),
		zap.String("userID", userID),
	)

	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		logger.L.Error("search query failed", zap.Error(err))
		return nil, err
	}
	defer rows.Close()

	var total int64
	if err := r.DB.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		logger.L.Warn("search count query failed", zap.Error(err))
		total = -1
	}

	items := []*model.FileSearchResult{}
	for rows.Next() {
		f := &model.FileSearchResult{}
		var createdAt, updatedAt time.Time
		var rank *float64
		if scanErr := rows.Scan(
			&f.ID, &f.Filename, &f.Mime, &f.Size,
			&createdAt, &updatedAt, &f.DownloadCount,
			&f.ContentHash, &f.PhysicalSize, &f.RefCount, &rank,
		); scanErr != nil {
			logger.L.Error("row scan failed", zap.Error(scanErr))
			return nil, errors.New("internal scan error")
		}

		f.CreatedAt = createdAt.Format(time.RFC3339)
		f.UpdatedAt = updatedAt.Format(time.RFC3339)
		f.DedupSavings = f.Size - f.PhysicalSize
		if rank != nil {
			f.Rank = rank
		}
		items = append(items, f)
	}

	return &model.FileSearchResponse{
		Items:  items,
		Total:  int(total),
		Limit:  p.Limit,
		Offset: p.Offset,
	}, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func defaultStr(s *string, def string) string {
	if s == nil || *s == "" {
		return def
	}
	return *s
}

func defaultInt(n *int, def int) int {
	if n == nil {
		return def
	}
	return *n
}

// Query returns generated.QueryResolver implementation.
func (r *Resolver) Query() generated.QueryResolver { return &queryResolver{r} }

type queryResolver struct{ *Resolver }
