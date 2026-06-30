// Package fuzzy provides loose string matching shared by the daemon's
// cross-project credential search and the MCP server's project resolution.
package fuzzy

import "strings"

// Normalize lowercases and strips non-alphanumerics so "Caddy Pay",
// "caddy-pay" and "caddypay" all compare equal.
func Normalize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Match reports whether query loosely matches any of targets. An empty query
// matches anything.
func Match(query string, targets ...string) bool {
	nq := Normalize(query)
	if nq == "" {
		return true
	}
	for _, t := range targets {
		if strings.Contains(Normalize(t), nq) {
			return true
		}
	}
	return false
}
