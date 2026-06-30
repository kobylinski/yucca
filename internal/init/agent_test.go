package init_test

import (
	"os"
	"path/filepath"
	"testing"

	yuccaInit "github.com/kobylinski/yucca/internal/init"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFileFlags_Empty(t *testing.T) {
	specs, err := yuccaInit.ParseFileFlags(nil)
	require.NoError(t, err)
	assert.Empty(t, specs)
}

func TestParseFileFlags_BareFiles(t *testing.T) {
	specs, err := yuccaInit.ParseFileFlags([]string{".env", "auth.json"})
	require.NoError(t, err)
	assert.Len(t, specs, 2)
	assert.Equal(t, ".env", specs[0].Path)
	assert.Empty(t, specs[0].Keys)
	assert.Equal(t, "auth.json", specs[1].Path)
	assert.Empty(t, specs[1].Keys)
}

func TestParseFileFlags_WithKeys(t *testing.T) {
	specs, err := yuccaInit.ParseFileFlags([]string{
		".env,APP_KEY,DB_PASSWORD",
		"auth.json,http-basic.composer.fluxui.dev.password",
	})
	require.NoError(t, err)
	assert.Len(t, specs, 2)
	assert.Equal(t, ".env", specs[0].Path)
	assert.Equal(t, []string{"APP_KEY", "DB_PASSWORD"}, specs[0].Keys)
	assert.Equal(t, "auth.json", specs[1].Path)
	assert.Equal(t, []string{"http-basic.composer.fluxui.dev.password"}, specs[1].Keys)
}

func TestParseFileFlags_MixedError(t *testing.T) {
	_, err := yuccaInit.ParseFileFlags([]string{".env", "auth.json,password"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot mix")
}

func TestIsDecisionMode(t *testing.T) {
	bare, _ := yuccaInit.ParseFileFlags([]string{".env"})
	assert.False(t, yuccaInit.IsDecisionMode(bare))

	withKeys, _ := yuccaInit.ParseFileFlags([]string{".env,APP_KEY"})
	assert.True(t, yuccaInit.IsDecisionMode(withKeys))

	assert.False(t, yuccaInit.IsDecisionMode(nil))
}

func TestRunAgentDiscovery_AutoDetect(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".env"), []byte("APP_KEY=base64key\nDB_HOST=localhost\nDB_PASSWORD=secret123\n"), 0600)

	output, err := yuccaInit.RunAgentDiscovery(dir, nil)
	require.NoError(t, err)
	assert.Contains(t, output, "# Yucca Discovery")
	assert.Contains(t, output, "## Detected Secrets")
	assert.Contains(t, output, "### .env")
	assert.Contains(t, output, "- APP_KEY")
	assert.Contains(t, output, "- DB_HOST")
	assert.Contains(t, output, "- DB_PASSWORD")
	// Values should be masked
	assert.NotContains(t, output, "secret123")
	// Should include next steps
	assert.Contains(t, output, "## Next Steps")
}

func TestRunAgentDiscovery_WithExtraFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".env"), []byte("APP_KEY=abc\n"), 0600)
	os.WriteFile(filepath.Join(dir, "custom.json"), []byte(`{"api_token": "sk-123", "name": "test"}`), 0600)

	extraFiles := []yuccaInit.FileSpec{{Path: "custom.json"}}
	output, err := yuccaInit.RunAgentDiscovery(dir, extraFiles)
	require.NoError(t, err)
	assert.Contains(t, output, "### .env")
	assert.Contains(t, output, "### custom.json")
	assert.Contains(t, output, "- api_token")
	assert.Contains(t, output, "- name")
}

func TestRunAgentDiscovery_ExtraFileNotFound(t *testing.T) {
	dir := t.TempDir()
	extraFiles := []yuccaInit.FileSpec{{Path: "nonexistent.json"}}
	_, err := yuccaInit.RunAgentDiscovery(dir, extraFiles)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent.json")
}

func TestRunAgentDiscovery_NoFiles(t *testing.T) {
	dir := t.TempDir()
	output, err := yuccaInit.RunAgentDiscovery(dir, nil)
	require.NoError(t, err)
	assert.Contains(t, output, "No credential files detected")
	assert.Contains(t, output, "## Next Steps")
	assert.Contains(t, output, "yucca init --agent --name")
}

func TestRunAgentDecision_WritesConfig(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".env"), []byte("APP_KEY=abc\nDB_HOST=localhost\n"), 0600)

	specs := []yuccaInit.FileSpec{
		{Path: ".env", Keys: []string{"APP_KEY"}},
	}
	output, err := yuccaInit.RunAgentDecision(dir, "", specs)
	require.NoError(t, err)

	// Check .yucca.yaml was written
	yaml, err := os.ReadFile(filepath.Join(dir, ".yucca.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(yaml), ".env")
	assert.Contains(t, string(yaml), "APP_KEY")
	// DB_HOST should NOT be in secrets (not selected)
	assert.NotContains(t, string(yaml), "DB_HOST")

	// Check output contains setup instructions
	assert.Contains(t, output, "# Yucca Initialized")
	assert.Contains(t, output, "yucca hook session-start")
	assert.Contains(t, output, "mcpServers")
}

func TestRunAgentDecision_CustomProjectName(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".env"), []byte("KEY=val\n"), 0600)

	specs := []yuccaInit.FileSpec{{Path: ".env", Keys: []string{"KEY"}}}
	output, err := yuccaInit.RunAgentDecision(dir, "my-project", specs)
	require.NoError(t, err)
	assert.Contains(t, output, "my-project")

	yaml, _ := os.ReadFile(filepath.Join(dir, ".yucca.yaml"))
	assert.Contains(t, string(yaml), "project_name: my-project")
}

func TestRunAgentDecision_DoesNotWriteClaudeFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".env"), []byte("KEY=val\n"), 0600)

	specs := []yuccaInit.FileSpec{{Path: ".env", Keys: []string{"KEY"}}}
	yuccaInit.RunAgentDecision(dir, "", specs)

	// .claude/settings.json and .mcp.json should NOT be created
	_, err := os.Stat(filepath.Join(dir, ".claude", "settings.json"))
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(dir, ".mcp.json"))
	assert.True(t, os.IsNotExist(err))
}

func TestRunAgentDecision_MultipleFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".env"), []byte("APP_KEY=abc\nDB_PASS=secret\n"), 0600)
	os.WriteFile(filepath.Join(dir, "auth.json"), []byte(`{"password": "s3cret"}`), 0600)

	specs := []yuccaInit.FileSpec{
		{Path: ".env", Keys: []string{"APP_KEY", "DB_PASS"}},
		{Path: "auth.json", Keys: []string{"password"}},
	}
	output, err := yuccaInit.RunAgentDecision(dir, "", specs)
	require.NoError(t, err)

	yaml, _ := os.ReadFile(filepath.Join(dir, ".yucca.yaml"))
	content := string(yaml)
	assert.Contains(t, content, ".env")
	assert.Contains(t, content, "auth.json")
	assert.Contains(t, content, "APP_KEY")
	assert.Contains(t, content, "DB_PASS")
	assert.Contains(t, content, "password")
	assert.Contains(t, output, "Protected files: 2")
	assert.Contains(t, output, "Secrets: 3")
}
