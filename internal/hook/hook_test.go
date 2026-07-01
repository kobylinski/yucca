package hook

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseHookInput(t *testing.T) {
	input := `{
		"session_id": "abc123",
		"cwd": "/Users/test/project",
		"hook_event_name": "PreToolUse",
		"tool_name": "Read",
		"tool_input": {"file_path": "/Users/test/project/.env"}
	}`
	hi, err := ParseHookInput(bytes.NewReader([]byte(input)))
	require.NoError(t, err)
	assert.Equal(t, "abc123", hi.SessionID)
	assert.Equal(t, "/Users/test/project", hi.Cwd)
	assert.Equal(t, "PreToolUse", hi.HookEventName)
	assert.Equal(t, "Read", hi.ToolName)
	assert.Equal(t, "/Users/test/project/.env", hi.ToolInput.FilePath)
}

func TestSessionStartOutput(t *testing.T) {
	out := SessionStartOutput("sess-1", "http://127.0.0.1:9777")
	var result map[string]any
	err := json.Unmarshal([]byte(out), &result)
	require.NoError(t, err)
	hso := result["hookSpecificOutput"].(map[string]any)
	assert.Equal(t, "SessionStart", hso["hookEventName"])
	ctx := hso["additionalContext"].(string)
	assert.Contains(t, ctx, "sess-1")
}

func TestSessionStartOutputMentionsRealTools(t *testing.T) {
	out := SessionStartOutput("session-123", "http://127.0.0.1:9777")
	// Advertises the real tools + the placeholder convention…
	assert.Contains(t, out, "yucca_secret_request")
	assert.Contains(t, out, "yucca_file")
	assert.Contains(t, out, "{{YUCCA:alias}}")
	// …and never names that don't exist (see mcp.TestSessionStartContextToolsExist).
	assert.NotContains(t, out, "yucca_credential_")
	assert.NotContains(t, out, "yucca_fs_")
}

func TestPreToolUseDeny(t *testing.T) {
	out := PreToolUseDeny("Use the yucca_file MCP tool instead")
	var result map[string]any
	err := json.Unmarshal([]byte(out), &result)
	require.NoError(t, err)
	hso := result["hookSpecificOutput"].(map[string]any)
	assert.Equal(t, "deny", hso["permissionDecision"])
}

func TestPreToolUseAllow(t *testing.T) {
	out := PreToolUseAllow()
	var result map[string]any
	err := json.Unmarshal([]byte(out), &result)
	require.NoError(t, err)
	hso := result["hookSpecificOutput"].(map[string]any)
	assert.Equal(t, "allow", hso["permissionDecision"])
}
