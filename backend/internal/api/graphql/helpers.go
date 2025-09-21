package graphql

// Small helpers used by resolvers. Kept separate so gqlgen updates don't clobber them.

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
