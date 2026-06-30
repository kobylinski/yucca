package scanner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetect_FindsEnvFiles(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, ".env"), []byte("KEY=val"), 0600)
	os.WriteFile(filepath.Join(dir, ".env.local"), []byte("KEY=val"), 0600)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0600)

	found, err := Detect(dir)
	require.NoError(t, err)

	paths := make([]string, len(found))
	for i, f := range found {
		paths[i] = f.Path
	}
	assert.Contains(t, paths, ".env")
	assert.Contains(t, paths, ".env.local")
	assert.NotContains(t, paths, "main.go")
}

func TestDetect_FindsDockerCompose(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "compose.yml"), []byte("services:"), 0600)

	found, err := Detect(dir)
	require.NoError(t, err)
	assert.Len(t, found, 1)
	assert.Equal(t, "compose.yml", found[0].Path)
	assert.Equal(t, "Docker", found[0].Category)
}

func TestDetect_FindsNestedFiles(t *testing.T) {
	dir := t.TempDir()

	os.MkdirAll(filepath.Join(dir, "config"), 0755)
	os.WriteFile(filepath.Join(dir, "config", "database.yml"), []byte("db: pg"), 0600)

	found, err := Detect(dir)
	require.NoError(t, err)
	assert.Len(t, found, 1)
	assert.Equal(t, "config/database.yml", found[0].Path)
}

func TestDetect_GlobPatterns(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "server.key"), []byte("KEY"), 0600)
	os.WriteFile(filepath.Join(dir, "cert.pem"), []byte("CERT"), 0600)

	found, err := Detect(dir)
	require.NoError(t, err)

	paths := make([]string, len(found))
	for i, f := range found {
		paths[i] = f.Path
	}
	assert.Contains(t, paths, "server.key")
	assert.Contains(t, paths, "cert.pem")
}

func TestDetect_EmptyProject(t *testing.T) {
	dir := t.TempDir()

	found, err := Detect(dir)
	require.NoError(t, err)
	assert.Len(t, found, 0)
}

func TestDetect_SkipsDirectories(t *testing.T) {
	dir := t.TempDir()

	// Create a directory named ".env" — should not be detected
	os.MkdirAll(filepath.Join(dir, ".env"), 0755)

	found, err := Detect(dir)
	require.NoError(t, err)
	assert.Len(t, found, 0)
}

func TestDetect_RecursesIntoSubdirs(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "apps", "web"), 0755)
	os.WriteFile(filepath.Join(dir, "apps", "web", ".env"), []byte("X=1"), 0600)
	os.MkdirAll(filepath.Join(dir, "node_modules", "pkg"), 0755)
	os.WriteFile(filepath.Join(dir, "node_modules", "pkg", ".env"), []byte("X=1"), 0600)

	found, err := Detect(dir)
	require.NoError(t, err)
	paths := make([]string, len(found))
	for i, f := range found {
		paths[i] = f.Path
	}
	assert.Contains(t, paths, filepath.Join("apps", "web", ".env"), "finds nested .env")
	assert.NotContains(t, paths, filepath.Join("node_modules", "pkg", ".env"), "skips node_modules")
}

func TestDetect_NoDuplicates(t *testing.T) {
	dir := t.TempDir()

	// .env matches the ".env" pattern — should appear only once
	os.WriteFile(filepath.Join(dir, ".env"), []byte("KEY=val"), 0600)

	found, err := Detect(dir)
	require.NoError(t, err)

	count := 0
	for _, f := range found {
		if f.Path == ".env" {
			count++
		}
	}
	assert.Equal(t, 1, count)
}
