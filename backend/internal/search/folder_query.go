package search

import (
	"fmt"
	"strings"
)

type FolderParams struct {
	UserID   string
	Q        string
	ParentID string // empty => root
	Limit    int
	Offset   int
	Sort     string // created_at_desc|created_at_asc|name_asc|name_desc
	// When true and ParentID is empty, do not restrict to root-only; search across all folders.
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
			// default behavior: only top-level folders
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
		// require pg_trgm for similarity; safe to keep as NULL if not available
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
