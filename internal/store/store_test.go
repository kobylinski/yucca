package store

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

func TestStore_SetAndGetCredential(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()
	s, err := New(dir)
	require.NoError(t, err)

	projectPath := "/Users/test/myproject"
	err = s.SetCredential(projectPath, "API_KEY", "secret123", PolicyAskSession)
	require.NoError(t, err)

	val, meta, err := s.GetCredential(projectPath, "API_KEY")
	require.NoError(t, err)
	assert.Equal(t, "secret123", val)
	assert.Equal(t, PolicyAskSession, meta.Policy)
	assert.Equal(t, "API_KEY", meta.Alias)
}

func TestStore_ListProjects(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()
	s, err := New(dir)
	require.NoError(t, err)

	_ = s.SetCredential("/path/a", "KEY1", "v1", PolicyAlwaysAllow)
	_ = s.SetCredential("/path/b", "KEY2", "v2", PolicyAskAlways)

	projects, err := s.ListProjects()
	require.NoError(t, err)
	assert.Len(t, projects, 2)
}

func TestStore_BorrowCredential(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()
	s, err := New(dir)
	require.NoError(t, err)

	_ = s.SetCredential("/path/source", "SHARED_KEY", "shared-secret", PolicyAskSession)

	matches, err := s.FindCredentialAcrossProjects("SHARED_KEY", "/path/target")
	require.NoError(t, err)
	assert.Len(t, matches, 1)
	assert.Equal(t, "/path/source", matches[0].Project.Path)
}

func TestStore_DeleteCredential(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()
	s, err := New(dir)
	require.NoError(t, err)

	_ = s.SetCredential("/path/a", "KEY", "val", PolicyAskSession)
	err = s.DeleteCredential("/path/a", "KEY")
	require.NoError(t, err)

	_, _, err = s.GetCredential("/path/a", "KEY")
	assert.Error(t, err)
}

func TestStore_SetCredentialWithSource(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()
	s, err := New(dir)
	require.NoError(t, err)

	err = s.SetCredentialWithSource("/path/proj", "DB_PASSWORD", "s3cret", PolicyAskSession, CredentialSource{
		Type:     "file",
		FilePath: ".env",
		FileKey:  "DB_PASSWORD",
	})
	require.NoError(t, err)

	val, meta, err := s.GetCredential("/path/proj", "DB_PASSWORD")
	require.NoError(t, err)
	assert.Equal(t, "s3cret", val)
	assert.Equal(t, "file", meta.Source.Type)
	assert.Equal(t, ".env", meta.Source.FilePath)
	assert.Equal(t, "DB_PASSWORD", meta.Source.FileKey)
}

func TestStore_SetCredential_DefaultsToLocal(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()
	s, err := New(dir)
	require.NoError(t, err)

	err = s.SetCredential("/path/proj", "KEY", "val", PolicyAskSession)
	require.NoError(t, err)

	_, meta, err := s.GetCredential("/path/proj", "KEY")
	require.NoError(t, err)
	assert.Equal(t, "local", meta.Source.Type)
}

func TestSetCredentialContext(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()
	s, err := New(dir)
	require.NoError(t, err)

	err = s.SetCredential("/tmp/testproject", "MY_KEY", "secret123", PolicyAlwaysAllow)
	require.NoError(t, err)

	err = s.SetCredentialContext("/tmp/testproject", "MY_KEY", "Used for Stripe test mode API.")
	require.NoError(t, err)

	_, meta, err := s.GetCredential("/tmp/testproject", "MY_KEY")
	require.NoError(t, err)
	assert.Equal(t, "Used for Stripe test mode API.", meta.Context)
}

func TestSetCredentialContext_NotFound(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()
	s, err := New(dir)
	require.NoError(t, err)

	err = s.SetCredentialContext("/tmp/testproject", "NONEXISTENT", "some context")
	assert.Error(t, err)
}

func TestSearchCredentials(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()
	s, err := New(dir)
	require.NoError(t, err)

	projectPath := "/tmp/searchtest"
	s.SetCredentialWithSource(projectPath, ".env:API_KEY", "key1", PolicyAlwaysAllow, CredentialSource{Type: "file", FilePath: ".env", FileKey: "API_KEY"})
	s.SetCredentialContext(projectPath, ".env:API_KEY", "Stripe test mode key")
	s.SetCredentialWithSource(projectPath, ".env:DB_PASSWORD", "pass1", PolicyAlwaysAllow, CredentialSource{Type: "file", FilePath: ".env", FileKey: "DB_PASSWORD"})
	s.SetCredentialContext(projectPath, ".env:DB_PASSWORD", "PostgreSQL production database")
	s.SetCredential(projectPath, "MANUAL_TOKEN", "tok1", PolicyAlwaysAllow)

	results := s.SearchCredentials(projectPath, "API")
	assert.Len(t, results, 1)
	assert.Equal(t, ".env:API_KEY", results[0].Alias)

	results = s.SearchCredentials(projectPath, "stripe")
	assert.Len(t, results, 1)

	results = s.SearchCredentials(projectPath, "env")
	assert.Len(t, results, 2)

	results = s.SearchCredentials(projectPath, "nonexistent")
	assert.Len(t, results, 0)
}

func TestCredentialSourceFileFields(t *testing.T) {
	src := CredentialSource{
		Type:     "file",
		FilePath: ".env",
		FileKey:  "API_KEY",
	}
	assert.Equal(t, "file", src.Type)
	assert.Equal(t, ".env", src.FilePath)
	assert.Equal(t, "API_KEY", src.FileKey)
}

func TestStore_HasCredential(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()
	s, err := New(dir)
	require.NoError(t, err)

	_ = s.SetCredential("/path/proj", "EXISTING_KEY", "val", PolicyAskSession)

	assert.True(t, s.HasCredential("/path/proj", "EXISTING_KEY"))
	assert.False(t, s.HasCredential("/path/proj", "NONEXISTENT"))
	assert.False(t, s.HasCredential("/path/other", "EXISTING_KEY"))
}

func TestValidateAlias(t *testing.T) {
	assert.NoError(t, ValidateAlias("MY_KEY"))
	assert.NoError(t, ValidateAlias("caddy-pay-API_KEY"))
	assert.NoError(t, ValidateAlias("some.dotted.name"))
	assert.NoError(t, ValidateAlias("a"))

	assert.Error(t, ValidateAlias(""))
	assert.Error(t, ValidateAlias("has spaces"))
	assert.Error(t, ValidateAlias("has:colon"))
	assert.Error(t, ValidateAlias("special@char"))
	assert.Error(t, ValidateAlias(strings.Repeat("x", 65)))
}

func TestStore_SetCredential_RejectsInvalidAlias(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()
	s, err := New(dir)
	require.NoError(t, err)

	err = s.SetCredential("/path/proj", "has spaces", "val", PolicyAskSession)
	assert.Error(t, err)

	err = s.SetCredential("/path/proj", "has:colon", "val", PolicyAskSession)
	assert.Error(t, err)

	// File source with colon should be allowed
	err = s.SetCredentialWithSource("/path/proj", ".env:API_KEY", "", PolicyAlwaysAllow, CredentialSource{Type: "file", FilePath: ".env", FileKey: "API_KEY"})
	assert.NoError(t, err)
}

func TestStore_ProjectSlug(t *testing.T) {
	s1 := projectSlug("/Users/test/myproject")
	s2 := projectSlug("/Users/test/myproject")
	s3 := projectSlug("/Users/test/other")
	assert.Equal(t, s1, s2)
	assert.NotEqual(t, s1, s3)
	assert.Equal(t, "-Users-test-myproject", s1)
}
