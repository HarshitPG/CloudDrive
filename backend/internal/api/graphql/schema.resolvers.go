package graphql

import (
	"backend/internal/api/graphql/generated"
	"backend/internal/api/graphql/model"
	"backend/internal/auth"
	"backend/internal/search"
	"backend/pkg/logger"
	"context"
	"database/sql"
	"fmt"
	"time"

	"go.uber.org/zap"
)

type Resolver struct{ DB *sql.DB }

type queryResolver struct{ *Resolver }

// Query returns generated.QueryResolver implementation.
func (r *Resolver) Query() generated.QueryResolver { return &queryResolver{r} }

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
		var (
			id, filename, mimeStr, contentHash string
			size, physicalSize, refCount       int64
			downloadCount                      int64
			createdAt, updatedAt               time.Time
			rank                               *float64
		)
		if scanErr := rows.Scan(
			&id, &filename, &mimeStr, &size,
			&createdAt, &updatedAt, &downloadCount,
			&contentHash, &physicalSize, &refCount, &rank,
		); scanErr != nil {
			logger.L.Error("row scan failed", zap.Error(scanErr))
			return nil, scanErr
		}

		f := &model.FileSearchResult{
			ID:            id,
			Filename:      filename,
			Mime:          mimeStr,
			Size:          int(size),
			CreatedAt:     createdAt.Format(time.RFC3339),
			UpdatedAt:     updatedAt.Format(time.RFC3339),
			DownloadCount: int(downloadCount),
			ContentHash:   contentHash,
			PhysicalSize:  int(physicalSize),
			RefCount:      int(refCount),
			DedupSavings:  int(size - physicalSize),
		}
		if rank != nil {
			f.Rank = rank
		}
		items = append(items, f)
	}

	return &model.FileSearchResponse{
		Items:  items,
		Total:  int(total),
		Limit:  lim,
		Offset: off,
	}, nil
}

// Search across files and folders, with optional scoping to a parent folder.
func (r *queryResolver) SearchItems(
	ctx context.Context,
	q *string,
	folderID *string,
	limit *int,
	offset *int,
	sort *string,
) (*model.CombinedSearchResponse, error) {
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
	sortVal := defaultStr(sort, "created_at_desc")
	qVal := deref(q)
	folder := deref(folderID)
	// For dashboard root search we want nested items too, so do not enforce rootOnly
	rootOnly := false

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Files
	fp := search.Params{
		UserID:        userID,
		Q:             qVal,
		FolderID:      folder,
		Sort:          sortVal,
		Limit:         lim,
		Offset:        off,
		IncludeRank:   qVal != "",
		IncludeShared: true,
	}
	fQuery, fCountQuery, fArgs := search.BuildQueryScoped(fp, rootOnly)

	// Folders
	dqp := search.FolderParams{
		UserID:   userID,
		Q:        qVal,
		ParentID: folder, // empty => root
		Limit:    lim,
		Offset:   off,
		Sort:     sortVal,
		AnyDepth: folder == "",
	}
	dQuery, dCountQuery, dArgs := search.BuildFolderQuery(dqp)

	// Execute queries
	files := []*model.FileSearchResult{}
	var totalFiles int64
	if rows, err := r.DB.QueryContext(ctx, fQuery, fArgs...); err == nil {
		for rows.Next() {
			var (
				id, filename, mimeStr, contentHash string
				size, physicalSize, refCount       int64
				downloadCount                      int64
				createdAt, updatedAt               time.Time
				rank                               *float64
			)
			if scanErr := rows.Scan(
				&id, &filename, &mimeStr, &size,
				&createdAt, &updatedAt, &downloadCount,
				&contentHash, &physicalSize, &refCount, &rank,
			); scanErr == nil {
				f := &model.FileSearchResult{
					ID:            id,
					Filename:      filename,
					Mime:          mimeStr,
					Size:          int(size),
					CreatedAt:     createdAt.Format(time.RFC3339),
					UpdatedAt:     updatedAt.Format(time.RFC3339),
					DownloadCount: int(downloadCount),
					ContentHash:   contentHash,
					PhysicalSize:  int(physicalSize),
					RefCount:      int(refCount),
					DedupSavings:  int(size - physicalSize),
				}
				if rank != nil {
					f.Rank = rank
				}
				files = append(files, f)
			}
		}
		rows.Close()
		_ = r.DB.QueryRowContext(ctx, fCountQuery, fArgs...).Scan(&totalFiles)
	} else {
		logger.L.Error("files search failed", zap.Error(err))
		return nil, err
	}

	folders := []*model.FolderSearchResult{}
	var totalFolders int64
	if rows, err := r.DB.QueryContext(ctx, dQuery, dArgs...); err == nil {
		for rows.Next() {
			var (
				id, name             string
				size                 int64
				createdAt, updatedAt time.Time
				rank                 *float64
			)
			if scanErr := rows.Scan(&id, &name, &size, &createdAt, &updatedAt, &rank); scanErr == nil {
				f := &model.FolderSearchResult{
					ID:        id,
					Name:      name,
					Size:      int(size),
					CreatedAt: createdAt.Format(time.RFC3339),
					UpdatedAt: updatedAt.Format(time.RFC3339),
				}
				if rank != nil {
					f.Rank = rank
				}
				folders = append(folders, f)
			}
		}
		rows.Close()
		_ = r.DB.QueryRowContext(ctx, dCountQuery, dArgs...).Scan(&totalFolders)
	} else {
		logger.L.Error("folders search failed", zap.Error(err))
		return nil, err
	}

	return &model.CombinedSearchResponse{
		Files:        files,
		Folders:      folders,
		TotalFiles:   int(totalFiles),
		TotalFolders: int(totalFolders),
		Limit:        lim,
		Offset:       off,
	}, nil
}
