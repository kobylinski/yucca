package proxy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRedact(t *testing.T) {
	secrets := map[string]string{
		"API_KEY": "sk-secret-123",
		"DB_PASS": "hunter2",
	}

	content := "API_KEY=sk-secret-123\nDB_PASS=hunter2\nPORT=3000"
	redacted := Redact(content, secrets)

	assert.NotContains(t, redacted, "sk-secret-123")
	assert.NotContains(t, redacted, "hunter2")
	assert.Contains(t, redacted, "{{YUCCA:API_KEY}}")
	assert.Contains(t, redacted, "{{YUCCA:DB_PASS}}")
	assert.Contains(t, redacted, "PORT=3000")
}

func TestRehydrate(t *testing.T) {
	secrets := map[string]string{
		"API_KEY": "sk-secret-123",
		"DB_PASS": "hunter2",
	}

	content := "API_KEY={{YUCCA:API_KEY}}\nDB_PASS={{YUCCA:DB_PASS}}\nPORT=3000"
	rehydrated := Rehydrate(content, secrets)

	assert.Contains(t, rehydrated, "sk-secret-123")
	assert.Contains(t, rehydrated, "hunter2")
	assert.Contains(t, rehydrated, "PORT=3000")
	assert.NotContains(t, rehydrated, "{{YUCCA:")
}

func TestRedactThenRehydrate_RoundTrip(t *testing.T) {
	secrets := map[string]string{
		"TOKEN": "ghp_abc123xyz",
	}
	original := "auth_token=ghp_abc123xyz\nhost=localhost"

	redacted := Redact(original, secrets)
	assert.NotContains(t, redacted, "ghp_abc123xyz")

	restored := Rehydrate(redacted, secrets)
	assert.Equal(t, original, restored)
}

func TestRedact_LongerSecretFirst(t *testing.T) {
	secrets := map[string]string{
		"SHORT": "abc",
		"LONG":  "abcdef",
	}
	content := "value=abcdef"
	redacted := Redact(content, secrets)
	// LONG should be replaced first since it's longer
	assert.Contains(t, redacted, "{{YUCCA:LONG}}")
	assert.NotContains(t, redacted, "{{YUCCA:SHORT}}")
}

func TestRehydrate_UnknownPlaceholder(t *testing.T) {
	secrets := map[string]string{"KEY": "val"}
	content := "a={{YUCCA:KEY}} b={{YUCCA:UNKNOWN}}"
	result := Rehydrate(content, secrets)
	assert.Contains(t, result, "val")
	assert.Contains(t, result, "{{YUCCA:UNKNOWN}}") // left as-is
}

func TestSanitize(t *testing.T) {
	secrets := map[string]string{
		"API_KEY": "sk-secret-123",
	}
	// Content that accidentally contains a raw secret
	content := "KEY={{YUCCA:API_KEY}}\nOTHER=sk-secret-123"
	sanitized := Sanitize(content, secrets)
	assert.Contains(t, sanitized, "{{YUCCA:API_KEY}}")
	// The raw occurrence should also be replaced
	assert.NotContains(t, sanitized, "sk-secret-123")
}
