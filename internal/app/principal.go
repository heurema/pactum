package app

import "strings"

// normalizePrincipal applies the uniform --by rule shared by every decision
// verb: trim whitespace, default an empty value to "manual", and sanitize
// repo-root absolute path text the same way memory acceptance does.
func normalizePrincipal(root string, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "manual"
	}
	return sanitizeRepoRootInText(root, value)
}
