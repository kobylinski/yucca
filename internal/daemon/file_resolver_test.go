package daemon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveFileCredential(t *testing.T) {
	projectDir := t.TempDir()
	envContent := "API_KEY=sk-test-12345\nDB_HOST=localhost\n"
	err := os.WriteFile(filepath.Join(projectDir, ".env"), []byte(envContent), 0600)
	require.NoError(t, err)

	val, err := ResolveFileCredential(projectDir, ".env", "API_KEY")
	require.NoError(t, err)
	assert.Equal(t, "sk-test-12345", val)
}

func TestResolveFileCredentialJSON(t *testing.T) {
	projectDir := t.TempDir()
	jsonContent := `{"database": {"password": "secret123", "host": "localhost"}}`
	err := os.WriteFile(filepath.Join(projectDir, "config.json"), []byte(jsonContent), 0600)
	require.NoError(t, err)

	val, err := ResolveFileCredential(projectDir, "config.json", "database.password")
	require.NoError(t, err)
	assert.Equal(t, "secret123", val)
}

func TestResolveFileCredentialMissing(t *testing.T) {
	projectDir := t.TempDir()
	os.WriteFile(filepath.Join(projectDir, ".env"), []byte("OTHER=val\n"), 0600)
	_, err := ResolveFileCredential(projectDir, ".env", "MISSING_KEY")
	assert.Error(t, err)
}
