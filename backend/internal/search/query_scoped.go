package search

// BuildQueryScoped wraps BuildQuery and enforces root-only when requested and no FolderID is provided.
func BuildQueryScoped(p Params, rootOnly bool) (string, string, []interface{}) {
	q, c, args := BuildQuery(p)
	if rootOnly && p.FolderID == "" {
		// Inject predicate into both queries; BuildQuery always has a WHERE clause
		q = injectBefore(q, "ORDER BY", " AND uf.folder_id IS NULL ")
		c = c + " AND uf.folder_id IS NULL "
	}
	return q, c, args
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
