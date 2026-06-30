package hook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandlePreToolUse_ReadProtectedFile(t *testing.T) {
	input := &HookInput{
		HookEventName: "PreToolUse",
		ToolName:      "Read",
		ToolInput:     ToolInput{FilePath: "/project/.env"},
		Cwd:           "/project",
	}
	protected := []string{".env", ".env.*", "*.tfvars"}
	result := HandlePreToolUse(input, protected)

	var out map[string]any
	err := json.Unmarshal([]byte(result), &out)
	require.NoError(t, err)
	hso := out["hookSpecificOutput"].(map[string]any)
	assert.Equal(t, "deny", hso["permissionDecision"])
}

func TestHandlePreToolUse_ReadNormalFile(t *testing.T) {
	input := &HookInput{
		HookEventName: "PreToolUse",
		ToolName:      "Read",
		ToolInput:     ToolInput{FilePath: "/project/main.go"},
		Cwd:           "/project",
	}
	protected := []string{".env", ".env.*", "*.tfvars"}
	result := HandlePreToolUse(input, protected)
	assert.Empty(t, result)
}

func TestHandlePreToolUse_WriteProtectedFile(t *testing.T) {
	input := &HookInput{
		HookEventName: "PreToolUse",
		ToolName:      "Write",
		ToolInput:     ToolInput{FilePath: "/project/.env.local"},
		Cwd:           "/project",
	}
	protected := []string{".env", ".env.*"}
	result := HandlePreToolUse(input, protected)

	var out map[string]any
	err := json.Unmarshal([]byte(result), &out)
	require.NoError(t, err)
	hso := out["hookSpecificOutput"].(map[string]any)
	assert.Equal(t, "deny", hso["permissionDecision"])
}

func TestHandlePreToolUse_BashCommand(t *testing.T) {
	input := &HookInput{
		HookEventName: "PreToolUse",
		ToolName:      "Bash",
		ToolInput:     ToolInput{Command: "npm test"},
		Cwd:           "/project",
	}
	protected := []string{".env"}
	result := HandlePreToolUse(input, protected)
	assert.Empty(t, result)
}

func TestHandlePreToolUse_BashGrepGate(t *testing.T) {
	protected := []string{".env", ".env.*", "secrets/**"}
	bash := func(cmd string) string {
		return HandlePreToolUse(&HookInput{ToolName: "Bash", Cwd: "/project", ToolInput: ToolInput{Command: cmd}}, protected)
	}
	// Casual reads of a protected file must be denied.
	assert.NotEmpty(t, bash("cat .env"), "cat .env")
	assert.NotEmpty(t, bash("cat ./.env"), "cat ./.env")
	assert.NotEmpty(t, bash("grep API_KEY .env"), "grep .env")
	assert.NotEmpty(t, bash("head -n 5 secrets/db.json"), "head secrets/")
	assert.NotEmpty(t, bash("cat < .env"), "redirect from .env")
	assert.NotEmpty(t, bash("API_FOO=1 cat .env"), "leading assignment then cat")
	// Legit commands that don't read a protected file are allowed.
	assert.Empty(t, bash("npm run dev"), "npm run dev")
	assert.Empty(t, bash("cat README.md"), "cat README")
	assert.Empty(t, bash("cat /etc/hosts"), "non-protected absolute path")
	assert.Empty(t, bash("docker compose --env-file .env up"), "docker isn't a reader verb")

	// Grep tool searching a protected file is denied.
	grepProtected := HandlePreToolUse(&HookInput{ToolName: "Grep", Cwd: "/project", ToolInput: ToolInput{Path: ".env", Pattern: "KEY"}}, protected)
	assert.NotEmpty(t, grepProtected)
	grepOK := HandlePreToolUse(&HookInput{ToolName: "Grep", Cwd: "/project", ToolInput: ToolInput{Path: "src", Pattern: "KEY"}}, protected)
	assert.Empty(t, grepOK)
}

func TestIsProtectedPath(t *testing.T) {
	patterns := []string{".env", ".env.*", "secrets/**", "*.tfvars"}

	assert.True(t, isProtectedPath("/project/.env", "/project", patterns))
	assert.True(t, isProtectedPath("/project/.env.local", "/project", patterns))
	assert.True(t, isProtectedPath("/project/.env.production", "/project", patterns))
	assert.True(t, isProtectedPath("/project/secrets/db.json", "/project", patterns))
	assert.True(t, isProtectedPath("/project/infra/main.tfvars", "/project", patterns))
	assert.False(t, isProtectedPath("/project/main.go", "/project", patterns))
	assert.False(t, isProtectedPath("/project/src/app.ts", "/project", patterns))

	// Bypasses that must NOT slip through:
	assert.True(t, isProtectedPath("/project/.ENV", "/project", patterns), "case-insensitive FS bypass")
	assert.True(t, isProtectedPath("/project/.Env", "/project", patterns))
	assert.True(t, isProtectedPath("/project/./secrets/db.json", "/project", patterns), "dot-segment bypass")
	assert.True(t, isProtectedPath("/project/x/../secrets/db.json", "/project", patterns), "dotdot-segment bypass")
	assert.True(t, isProtectedPath("/project/./.env", "/project", patterns))
}

func TestLoadProtectedPatterns_FromFile(t *testing.T) {
	dir := t.TempDir()
	yaml := "protected_files:\n  - auth.json\n  - config/secrets.yaml\n"
	os.WriteFile(filepath.Join(dir, ".yucca.yaml"), []byte(yaml), 0600)

	patterns := LoadProtectedPatterns(dir)
	assert.Contains(t, patterns, "auth.json")
	assert.Contains(t, patterns, "config/secrets.yaml")
	// Defaults should still be included
	assert.Contains(t, patterns, ".env")
}

func TestLoadProtectedPatterns_NoFile(t *testing.T) {
	patterns := LoadProtectedPatterns(t.TempDir())
	assert.Equal(t, DefaultProtectedPatterns, patterns)
}
