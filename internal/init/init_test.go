package init_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	yuccaInit "github.com/kobylinski/yucca/internal/init"
	"github.com/kobylinski/yucca/internal/scanner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteYuccaYAML(t *testing.T) {
	dir := t.TempDir()
	files := []string{".env", "config/secrets.json"}
	err := yuccaInit.WriteYuccaYAML(dir, "testproject", files)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, ".yucca.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(data), ".env")
	assert.Contains(t, string(data), "config/secrets.json")
	assert.Contains(t, string(data), "project_name: testproject")
}

func TestInstallClaudeHooks(t *testing.T) {
	dir := t.TempDir()
	err := yuccaInit.InstallClaudeHooks(dir)
	require.NoError(t, err)

	// Check hooks in settings.json
	data, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "yucca hook session-start")
	assert.Contains(t, string(data), "yucca hook pre-tool-use")
	assert.Contains(t, string(data), "yucca hook session-end")
	assert.NotContains(t, string(data), "mcpServers") // MCP goes in .mcp.json

	// Check .mcp.json — must have mcpServers wrapper with type/env
	mcpData, err := os.ReadFile(filepath.Join(dir, ".mcp.json"))
	require.NoError(t, err)
	mcpContent := string(mcpData)
	assert.Contains(t, mcpContent, `"mcpServers"`)
	assert.Contains(t, mcpContent, `"yucca"`)
	assert.Contains(t, mcpContent, `"type": "stdio"`)
	assert.Contains(t, mcpContent, `"command": "yucca"`)
	assert.Contains(t, mcpContent, `"mcp"`)
	assert.Contains(t, mcpContent, `"env"`)
}

func TestInstallClaudeHooks_PreservesExisting(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".claude"), 0755)

	// Existing settings with permissions and a custom hook
	existing := map[string]any{
		"permissions": map[string]any{"allow": []string{"Read"}},
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{
					"matcher": "Bash",
					"hooks": []any{
						map[string]any{"type": "command", "command": "my-custom-hook"},
					},
				},
			},
		},
	}
	raw, _ := json.MarshalIndent(existing, "", "  ")
	os.WriteFile(filepath.Join(dir, ".claude", "settings.json"), raw, 0600)

	err := yuccaInit.InstallClaudeHooks(dir)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	require.NoError(t, err)
	content := string(data)

	// Existing keys preserved
	assert.Contains(t, content, "permissions")
	assert.Contains(t, content, "my-custom-hook")

	// Yucca hooks added
	assert.Contains(t, content, "yucca hook session-start")
	assert.Contains(t, content, "yucca hook pre-tool-use")
}

func TestInstallClaudeHooks_Idempotent(t *testing.T) {
	dir := t.TempDir()

	// Run twice
	require.NoError(t, yuccaInit.InstallClaudeHooks(dir))
	require.NoError(t, yuccaInit.InstallClaudeHooks(dir))

	data, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	require.NoError(t, err)

	// Parse and check no duplicate yucca hooks
	var settings map[string]any
	require.NoError(t, json.Unmarshal(data, &settings))

	hooks := settings["hooks"].(map[string]any)
	sessionStart := hooks["SessionStart"].([]any)
	yuccaCount := 0
	for _, entry := range sessionStart {
		if m, ok := entry.(map[string]any); ok {
			if hooksArr, ok := m["hooks"].([]any); ok {
				for _, h := range hooksArr {
					if hm, ok := h.(map[string]any); ok {
						if cmd, ok := hm["command"].(string); ok && cmd == "yucca hook session-start" {
							yuccaCount++
						}
					}
				}
			}
		}
	}
	assert.Equal(t, 1, yuccaCount, "should have exactly one yucca session-start hook")
}

func TestLoadProtectedFiles(t *testing.T) {
	dir := t.TempDir()
	yaml := "protected_files:\n  - .env\n  - config/secrets.yaml\n"
	os.WriteFile(filepath.Join(dir, ".yucca.yaml"), []byte(yaml), 0600)

	files := yuccaInit.LoadProtectedFiles(dir)
	assert.Contains(t, files, ".env")
	assert.Contains(t, files, "config/secrets.yaml")
}

func TestLoadProtectedFiles_NoFile(t *testing.T) {
	files := yuccaInit.LoadProtectedFiles(t.TempDir())
	assert.Nil(t, files)
}

func TestAddProtectedFile(t *testing.T) {
	dir := t.TempDir()
	yuccaInit.WriteYuccaYAML(dir, "test", []string{".env"})

	err := yuccaInit.AddProtectedFile(dir, "secrets.json")
	require.NoError(t, err)

	files := yuccaInit.LoadProtectedFiles(dir)
	assert.Contains(t, files, ".env")
	assert.Contains(t, files, "secrets.json")
}

func TestAddProtectedFile_Duplicate(t *testing.T) {
	dir := t.TempDir()
	yuccaInit.WriteYuccaYAML(dir, "test", []string{".env"})

	err := yuccaInit.AddProtectedFile(dir, ".env")
	require.NoError(t, err)

	files := yuccaInit.LoadProtectedFiles(dir)
	assert.Len(t, files, 1)
}

func TestComputeJSONDiff_NoChange(t *testing.T) {
	old := []byte(`{"mcpServers": {"yucca": {"command": "yucca"}}}`)
	new := []byte(`{"mcpServers": {"yucca": {"command": "yucca"}}}`)
	changed, _, _ := yuccaInit.ComputeJSONDiff(old, new)
	assert.False(t, changed)
}

func TestComputeJSONDiff_WithChange(t *testing.T) {
	old := []byte(`{"mcpServers": {"other": {"command": "other"}}}`)
	new := []byte(`{"mcpServers": {"other": {"command": "other"}, "yucca": {"command": "yucca"}}}`)
	changed, oldFormatted, newFormatted := yuccaInit.ComputeJSONDiff(old, new)
	assert.True(t, changed)
	assert.Contains(t, oldFormatted, "other")
	assert.Contains(t, newFormatted, "yucca")
}

func TestComputeJSONDiff_NewFile(t *testing.T) {
	changed, _, newFormatted := yuccaInit.ComputeJSONDiff(nil, []byte(`{"key": "value"}`))
	assert.True(t, changed)
	assert.Contains(t, newFormatted, "key")
}

func TestFormatDiffView_NewFile(t *testing.T) {
	output := yuccaInit.FormatDiffView("test.json", "", `{"key": "value"}`)
	assert.Contains(t, output, "test.json")
	assert.Contains(t, output, "new file")
	assert.Contains(t, output, "key")
}

func TestFormatDiffView_Modified(t *testing.T) {
	old := `{"existing": "value"}`
	new := `{"existing": "value", "added": "new"}`
	output := yuccaInit.FormatDiffView("test.json", old, new)
	assert.Contains(t, output, "test.json")
	assert.Contains(t, output, "modified")
	assert.Contains(t, output, "added")
}

func TestPrepareConfigChanges_ShowsDiffOnChange(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".claude"), 0755)

	existing := map[string]any{
		"permissions": map[string]any{"allow": []string{"Read"}},
	}
	raw, _ := json.MarshalIndent(existing, "", "  ")
	os.WriteFile(filepath.Join(dir, ".claude", "settings.json"), raw, 0600)

	changes, err := yuccaInit.PrepareConfigChanges(dir)
	require.NoError(t, err)
	assert.True(t, len(changes) > 0)
	for _, c := range changes {
		assert.NotEmpty(t, c.FilePath)
		assert.NotEmpty(t, c.NewContent)
	}
}

func TestPrepareConfigChanges_SkipsWhenAlreadyConfigured(t *testing.T) {
	dir := t.TempDir()

	changes, _ := yuccaInit.PrepareConfigChanges(dir)
	yuccaInit.ApplyConfigChanges(changes)

	changes2, _ := yuccaInit.PrepareConfigChanges(dir)
	hasChanges := false
	for _, c := range changes2 {
		if c.Changed {
			hasChanges = true
		}
	}
	assert.False(t, hasChanges, "second run should detect no changes needed")
}

func TestPrepareConfigChanges_MCPSkipsExistingYucca(t *testing.T) {
	dir := t.TempDir()

	// Pre-existing .mcp.json with yucca already configured (by claude mcp add)
	existing := map[string]any{
		"mcpServers": map[string]any{
			"yucca": map[string]any{
				"type":    "stdio",
				"command": "yucca",
				"args":    []any{"mcp", "serve"},
				"env":     map[string]any{},
			},
		},
	}
	raw, _ := json.MarshalIndent(existing, "", "  ")
	os.WriteFile(filepath.Join(dir, ".mcp.json"), raw, 0600)

	changes, _ := yuccaInit.PrepareConfigChanges(dir)
	// Find the MCP change
	for _, c := range changes {
		if c.FileName == ".mcp.json" {
			assert.False(t, c.Changed, ".mcp.json should not be changed when yucca already exists")
		}
	}
}

func TestDeterministicAliasNaming(t *testing.T) {
	secrets := []yuccaInit.SelectedSecret{
		{File: ".env", Key: "API_KEY"},
		{File: "config/database.yml", Key: "production.password"},
	}

	aliases := yuccaInit.BuildAliases(secrets)
	assert.Equal(t, ".env:API_KEY", aliases[0])
	assert.Equal(t, "config/database.yml:production.password", aliases[1])
}

func TestReinitPreSelectsExistingSecrets(t *testing.T) {
	fields := []scanner.ParsedField{
		{Key: "API_KEY", Value: "sk-123"},
		{Key: "DB_HOST", Value: "localhost"},
		{Key: "DB_PASS", Value: "secret"},
	}

	existing := []yuccaInit.SelectedSecret{
		{File: ".env", Key: "API_KEY"},
		{File: ".env", Key: "DB_PASS"},
	}

	m := yuccaInit.NewFieldModelWithPreselection(".env", fields, existing)
	assert.True(t, m.IsSelected(0))  // API_KEY pre-selected
	assert.False(t, m.IsSelected(1)) // DB_HOST not selected
	assert.True(t, m.IsSelected(2))  // DB_PASS pre-selected
}

func TestWriteAndLoadSecrets(t *testing.T) {
	dir := t.TempDir()
	yuccaInit.WriteYuccaYAML(dir, "test", []string{".env", "config.json"})

	secrets := []yuccaInit.SelectedSecret{
		{File: ".env", Key: "API_KEY"},
		{File: ".env", Key: "DB_PASSWORD"},
		{File: "config.json", Key: "database.password"},
	}
	err := yuccaInit.WriteSecretsConfig(dir, secrets)
	require.NoError(t, err)

	loaded := yuccaInit.LoadSecrets(dir)
	assert.Len(t, loaded, 3)
	assert.Equal(t, ".env", loaded[0].File)
	assert.Equal(t, "API_KEY", loaded[0].Key)
	assert.Equal(t, "config.json", loaded[2].File)
	assert.Equal(t, "database.password", loaded[2].Key)
}
