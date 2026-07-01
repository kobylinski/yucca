package mcp

import (
	"regexp"
	"testing"

	"github.com/kobylinski/yucca/internal/hook"
	"github.com/stretchr/testify/assert"
)

// TestSessionStartContextToolsExist guards the SessionStart hook's injected
// context from drifting away from the real MCP toolset: every yucca_* tool it
// advertises to the agent must be a tool the server actually exposes. (It used
// to name yucca_fs_read / yucca_credential_search etc., which never existed.)
func TestSessionStartContextToolsExist(t *testing.T) {
	s := New("http://127.0.0.1:9777", "/tmp/test", nil)
	real := map[string]bool{}
	for _, td := range s.toolDefinitions() {
		real[td["name"].(string)] = true
	}

	ctx := hook.SessionStartOutput("sess-1", "http://127.0.0.1:9777")
	mentioned := regexp.MustCompile(`yucca_[a-z_]+`).FindAllString(ctx, -1)

	assert.NotEmpty(t, mentioned, "SessionStart context should reference at least one tool")
	for _, name := range mentioned {
		assert.Truef(t, real[name], "SessionStart context advertises unknown tool %q", name)
	}
}
