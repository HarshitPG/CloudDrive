package graphql

// THIS CODE WILL BE UPDATED WITH SCHEMA CHANGES. PREVIOUS IMPLEMENTATION FOR SCHEMA CHANGES WILL BE KEPT IN THE COMMENT SECTION. IMPLEMENTATION FOR UNCHANGED SCHEMA WILL BE KEPT.

import (
	"backend/internal/api/graphql/generated"
	"backend/internal/api/graphql/model"
	"backend/internal/auth"
	"backend/internal/search"
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"
)

type Resolver struct{ DB *sql.DB }

func (r *queryResolver) SearchFiles(
	ctx context.Context,
	q *string, mime *string,
	minSize *int, maxSize *int,
	dateFrom *string, dateTo *string,
	tags []string, uploader *string, folderID *string,
	limit *int, offset *int, sort *string,
) (*model.FileSearchResponse, error) {
	userID := auth.GetUserIDFromCtx(ctx)
	fmt.Printf("userid: %s\n", userID)
	if userID == "" {
		return nil, fmt.Errorf("unauthenticated")
	}

	qVal := deref(q)
	p := search.Params{
		UserID:        userID,
		Q:             qVal,
		Mime:          deref(mime),
		FolderID:      deref(folderID),
		Uploader:      deref(uploader),
		Sort:          defaultStr(sort, "created_at_desc"),
		Limit:         defaultInt(limit, 50),
		Offset:        defaultInt(offset, 0),
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
	log.Printf("SEARCH query=%s args=%v includeRank=%v", query, args, p.IncludeRank)

	rowsCh := make(chan *sql.Rows, 1)
	errCh := make(chan error, 2)
	countCh := make(chan int64, 1)

	go func() {
		rows, err := r.DB.QueryContext(ctx, query, args...)
		if err != nil {
			errCh <- err
			return
		}
		rowsCh <- rows
	}()

	go func() {
		var total int64
		if err := r.DB.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
			errCh <- err
			return
		}
		countCh <- total
	}()

	var rows *sql.Rows
	select {
	case rows = <-rowsCh:
	case err := <-errCh:
		return nil, err
	}
	defer rows.Close()

	var total int64
	select {
	case total = <-countCh:
	case err := <-errCh:
		return nil, err
	}

	items := []*model.FileSearchResult{}
	for rows.Next() {
		f := &model.FileSearchResult{}
		var createdAt, updatedAt time.Time
		var rank *float64
		err := rows.Scan(
			&f.ID, &f.Filename, &f.Mime, &f.Size,
			&createdAt, &updatedAt, &f.DownloadCount,
			&f.ContentHash, &f.PhysicalSize, &f.RefCount, &rank,
		)
		if err != nil {
			continue
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
