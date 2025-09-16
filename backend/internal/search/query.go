package search

import (
	"fmt"
	"strings"
	"time"
)

type Params struct {
	UserID      string
	Q           string
	Mime        string
	MinSize     *int64
	MaxSize     *int64
	DateFrom    *time.Time
	DateTo      *time.Time
	Tags        []string
	Uploader    string
	FolderID    string
	Limit       int
	Offset      int
	Sort        string
	IncludeRank bool
	//(per-user shares).
	IncludeShared bool
}

// BuildQuery builds SQL for search with filters and prefix search
func BuildQuery(p Params) (string, string, []interface{}) {
	where := []string{"uf.deleted_at IS NULL"}
	args := []interface{}{}
	argIdx := 1

	if p.UserID != "" {
		where = append(where,
			fmt.Sprintf("(uf.user_id = $%d OR EXISTS (SELECT 1 FROM share_users su JOIN shares s ON su.share_id = s.id WHERE s.target_type='file' AND s.revoked=false AND s.target_id = uf.id AND su.target_user_id = $%d))",
				argIdx, argIdx),
		)
		args = append(args, p.UserID)
		argIdx++
	}

	if p.FolderID != "" {
		where = append(where, fmt.Sprintf("uf.folder_id = $%d", argIdx))
		args = append(args, p.FolderID)
		argIdx++
	}

	if p.Mime != "" {
		where = append(where, fmt.Sprintf("uf.declared_mime = $%d", argIdx))
		args = append(args, p.Mime)
		argIdx++
	}

	if p.MinSize != nil {
		where = append(where, fmt.Sprintf("uf.original_size_bytes >= $%d", argIdx))
		args = append(args, *p.MinSize)
		argIdx++
	}
	if p.MaxSize != nil {
		where = append(where, fmt.Sprintf("uf.original_size_bytes <= $%d", argIdx))
		args = append(args, *p.MaxSize)
		argIdx++
	}

	if p.DateFrom != nil {
		where = append(where, fmt.Sprintf("uf.created_at >= $%d", argIdx))
		args = append(args, *p.DateFrom)
		argIdx++
	}
	if p.DateTo != nil {
		where = append(where, fmt.Sprintf("uf.created_at <= $%d", argIdx))
		args = append(args, *p.DateTo)
		argIdx++
	}

	for _, t := range p.Tags {
		where = append(where, fmt.Sprintf("uf.tags @> $%d::jsonb", argIdx))
		args = append(args, fmt.Sprintf(`["%s"]`, t))
		argIdx++
	}

	joinUsers := ""
	if p.Uploader != "" {
		joinUsers = " JOIN users u ON u.id = uf.user_id "
		where = append(where, fmt.Sprintf("to_tsvector('simple', coalesce(u.name,'')) @@ websearch_to_tsquery('simple', $%d)", argIdx))
		args = append(args, p.Uploader)
		argIdx++
	}

	rankSelect := ", NULL as rank"
	orderRank := ""
	if p.Q != "" {
		where = append(where, fmt.Sprintf(
			"(uf.search_document @@ websearch_to_tsquery('simple', $%d) OR uf.filename ILIKE $%d)",
			argIdx, argIdx+1,
		))
		args = append(args, p.Q, "%"+p.Q+"%")
		rankSelect = fmt.Sprintf(", ts_rank_cd(uf.search_document, websearch_to_tsquery('simple',$%d)) as rank", argIdx)
		orderRank = "rank DESC,"
		argIdx += 2
	} else {
		rankSelect = ", NULL as rank"
	}

	orderBy := "uf.created_at DESC"
	switch strings.ToLower(p.Sort) {
	case "created_at_asc":
		orderBy = "uf.created_at ASC"
	case "size_asc":
		orderBy = "uf.original_size_bytes ASC"
	case "size_desc":
		orderBy = "uf.original_size_bytes DESC"
	case "filename_asc":
		orderBy = "uf.filename ASC"
	case "filename_desc":
		orderBy = "uf.filename DESC"
	}

	limit := p.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := p.Offset
	if offset < 0 {
		offset = 0
	}

	whereSQL := ""
	if len(where) > 0 {
		whereSQL = "WHERE " + strings.Join(where, " AND ")
	}

	query := fmt.Sprintf(`
SELECT
  uf.id, uf.filename, uf.declared_mime, uf.original_size_bytes,
  uf.created_at, uf.updated_at, uf.download_count,
  fc.content_hash, fc.size_bytes, fc.ref_count %s
FROM user_files uf
JOIN file_contents fc ON uf.content_id = fc.id
%s
%s
ORDER BY %s %s
LIMIT %d OFFSET %d
`, rankSelect, joinUsers, whereSQL, orderRank, orderBy, limit, offset)

	countQuery := fmt.Sprintf("SELECT COUNT(1) FROM user_files uf %s %s", joinUsers, whereSQL)

	return query, countQuery, args
}
