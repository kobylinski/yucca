package proxy

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var placeholderRegex = regexp.MustCompile(`\{\{YUCCA:([^}]+)\}\}`)

// Redact replaces known secret values with {{YUCCA:alias}} placeholders.
// Longer values are replaced first to handle overlapping substrings.
func Redact(content string, secrets map[string]string) string {
	type kv struct{ alias, value string }
	sorted := make([]kv, 0, len(secrets))
	for alias, val := range secrets {
		if val != "" {
			sorted = append(sorted, kv{alias, val})
		}
	}
	sort.Slice(sorted, func(i, j int) bool {
		return len(sorted[i].value) > len(sorted[j].value)
	})

	for _, s := range sorted {
		placeholder := fmt.Sprintf("{{YUCCA:%s}}", s.alias)
		content = strings.ReplaceAll(content, s.value, placeholder)
	}
	return content
}

// Rehydrate replaces {{YUCCA:alias}} placeholders with actual secret values.
// Unknown placeholders are left as-is.
func Rehydrate(content string, secrets map[string]string) string {
	return placeholderRegex.ReplaceAllStringFunc(content, func(match string) string {
		subs := placeholderRegex.FindStringSubmatch(match)
		if len(subs) < 2 {
			return match
		}
		alias := subs[1]
		if val, ok := secrets[alias]; ok {
			return val
		}
		return match // unknown placeholder, leave as-is
	})
}

// Sanitize replaces any raw secret values that appear in content with
// their placeholder form. Use this before writing to ensure no secrets
// leak through even if the agent included raw values.
func Sanitize(content string, secrets map[string]string) string {
	return Redact(content, secrets)
}
