package search

import (
	"fmt"
	"strings"
	"time"
)

// Params defines file search parameters.
type Params struct {
	UserID        string
	Q             string
	Mime          string
	MinSize       *int64
	MaxSize       *int64
	DateFrom      *time.Time
	DateTo        *time.Time
	Tags          []string
	Uploader      string
	FolderID      string
	Limit         int
	Offset        int
	Sort          string
	IncludeRank   bool
	IncludeShared bool
}

func BuildQuery(p Params) (string, string, []interface{}) {
	where := []string{"uf.deleted_at IS NULL"}
	args := []interface{}{}
	argIdx := 1

	if p.UserID != "" {
		where = append(where,
			fmt.Sprintf(
				"(uf.user_id = $%d OR EXISTS (SELECT 1 FROM share_users su JOIN shares s ON su.share_id = s.id WHERE s.target_type='file' AND s.revoked=false AND s.target_id = uf.id AND su.target_user_id = $%d))",
				argIdx, argIdx,
			),
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
		if strings.HasSuffix(p.Mime, "/") {
			where = append(where, fmt.Sprintf("uf.declared_mime ILIKE $%d", argIdx))
			args = append(args, p.Mime+"%")
			argIdx++
		} else {
			// Map some common filter MIME values to a group of real MIME types
			mimeGroups := map[string][]string{
				"application/vnd.ms-powerpoint": {
					"application/vnd.ms-powerpoint",
					"application/vnd.openxmlformats-officedocument.presentationml.presentation",
				},
				"application/msword": {
					"application/msword",
					"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
				},
				"application/vnd.ms-excel": {
					"application/vnd.ms-excel",
					"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
				},
			}

			if group, ok := mimeGroups[p.Mime]; ok {
				parts := []string{}
				for _, m := range group {
					parts = append(parts, fmt.Sprintf("uf.declared_mime = $%d", argIdx))
					args = append(args, m)
					argIdx++
				}
				where = append(where, "("+strings.Join(parts, " OR ")+")")
			} else {
				where = append(where, fmt.Sprintf("uf.declared_mime = $%d", argIdx))
				args = append(args, p.Mime)
				argIdx++
			}
		}
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
		where = append(where, fmt.Sprintf(
			"to_tsvector('simple', coalesce(u.name,'')) @@ websearch_to_tsquery('simple', $%d)",
			argIdx,
		))
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

func BuildQueryScoped(p Params, rootOnly bool) (string, string, []interface{}) {
	q, c, args := BuildQuery(p)
	if rootOnly && p.FolderID == "" {
		q = injectBefore(q, "ORDER BY", " AND uf.folder_id IS NULL ")
		c = c + " AND uf.folder_id IS NULL "
	}
	return q, c, args
}

type FolderParams struct {
	UserID   string
	Q        string
	ParentID string
	Limit    int
	Offset   int
	Sort     string
	AnyDepth bool
}

func BuildFolderQuery(p FolderParams) (string, string, []interface{}) {
	where := []string{"f.deleted_at IS NULL"}
	args := []interface{}{}
	idx := 1

	if p.UserID != "" {
		where = append(where, fmt.Sprintf(`(
            f.user_id = $%d OR EXISTS (
                SELECT 1 FROM share_users su JOIN shares s ON su.share_id = s.id
                WHERE s.target_type='folder' AND s.revoked=false AND s.target_id=f.id AND su.target_user_id=$%d
            )
        )`, idx, idx))
		args = append(args, p.UserID)
		idx++
	}

	if p.ParentID == "" {
		if !p.AnyDepth {
			where = append(where, "f.parent_id IS NULL")
		}
	} else {
		where = append(where, fmt.Sprintf("f.parent_id = $%d", idx))
		args = append(args, p.ParentID)
		idx++
	}

	rankSel := ", NULL as rank"
	orderRank := ""
	if p.Q != "" {
		where = append(where, fmt.Sprintf("f.name ILIKE $%d", idx))
		args = append(args, "%"+p.Q+"%")
		rankSel = fmt.Sprintf(", similarity(lower(f.name), lower($%d)) as rank", idx)
		orderRank = "rank DESC,"
		idx++
	}

	orderBy := "f.created_at DESC"
	switch strings.ToLower(p.Sort) {
	case "created_at_asc":
		orderBy = "f.created_at ASC"
	case "name_asc":
		orderBy = "f.name ASC"
	case "name_desc":
		orderBy = "f.name DESC"
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
  f.id, f.name,
  COALESCE((SELECT SUM(fc.size_bytes)
            FROM user_files uf JOIN file_contents fc ON uf.content_id=fc.id
            WHERE uf.folder_id=f.id AND uf.deleted_at IS NULL), 0) AS size,
  f.created_at, f.updated_at %s
FROM folders f
%s
ORDER BY %s %s
LIMIT %d OFFSET %d
`, rankSel, whereSQL, orderRank, orderBy, limit, offset)

	countQuery := fmt.Sprintf("SELECT COUNT(1) FROM folders f %s", whereSQL)
	return query, countQuery, args
}

func injectBefore(s, token, snippet string) string {
	i := indexOf(s, token)
	if i < 0 {
		return s + snippet
	}
	return s[:i] + snippet + s[i:]
}

func indexOf(s, token string) int {
	for i := 0; i+len(token) <= len(s); i++ {
		if s[i:i+len(token)] == token {
			return i
		}
	}
	return -1
}
