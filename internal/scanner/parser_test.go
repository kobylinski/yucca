package scanner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseEnvFile(t *testing.T) {
	dir := t.TempDir()
	content := `# Database config
DB_HOST=localhost
DB_PASSWORD="super-secret"
API_KEY='sk-proj-abc123'
export REDIS_URL=redis://localhost:6379
PORT=3000
`
	path := filepath.Join(dir, ".env")
	os.WriteFile(path, []byte(content), 0600)

	fields, err := ParseFile(path)
	require.NoError(t, err)
	assert.Len(t, fields, 5)

	m := fieldMap(fields)
	assert.Equal(t, "localhost", m["DB_HOST"])
	assert.Equal(t, "super-secret", m["DB_PASSWORD"])
	assert.Equal(t, "sk-proj-abc123", m["API_KEY"])
	assert.Equal(t, "redis://localhost:6379", m["REDIS_URL"])
	assert.Equal(t, "3000", m["PORT"])
}

func TestParseEnvFile_Local(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env.local")
	os.WriteFile(path, []byte("KEY=val\n"), 0600)

	fields, err := ParseFile(path)
	require.NoError(t, err)
	assert.Len(t, fields, 1)
	assert.Equal(t, "KEY", fields[0].Key)
}

func TestParseJSONFile_PreservesLargeNumbers(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	os.WriteFile(p, []byte(`{"id": 1234567890123456789, "n": 1000000}`), 0600)
	fields, err := ParseFile(p)
	require.NoError(t, err)
	m := map[string]string{}
	for _, f := range fields {
		m[f.Key] = f.Value
	}
	assert.Equal(t, "1234567890123456789", m["id"], "large int kept exact")
	assert.Equal(t, "1000000", m["n"], "not turned into 1e+06")
}

func TestParseJSONFile(t *testing.T) {
	dir := t.TempDir()
	content := `{
		"database": {
			"host": "localhost",
			"password": "db-secret-pass"
		},
		"api_key": "sk-123",
		"port": 3000
	}`
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(content), 0600)

	fields, err := ParseFile(path)
	require.NoError(t, err)

	m := fieldMap(fields)
	assert.Equal(t, "localhost", m["database.host"])
	assert.Equal(t, "db-secret-pass", m["database.password"])
	assert.Equal(t, "sk-123", m["api_key"])
	assert.Equal(t, "3000", m["port"])
}

func TestParseJSONFile_WithArrays(t *testing.T) {
	dir := t.TempDir()
	content := `{"hosts": ["a", "b"], "db": {"pass": "secret"}}`
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(content), 0600)

	fields, err := ParseFile(path)
	require.NoError(t, err)

	m := fieldMap(fields)
	assert.Equal(t, "a", m["hosts[0]"])
	assert.Equal(t, "b", m["hosts[1]"])
	assert.Equal(t, "secret", m["db.pass"])
}

func TestParseYAMLFile(t *testing.T) {
	dir := t.TempDir()
	content := `database:
  host: localhost
  password: yaml-secret
api_key: sk-yaml-123
port: 5432
`
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte(content), 0600)

	fields, err := ParseFile(path)
	require.NoError(t, err)

	m := fieldMap(fields)
	assert.Equal(t, "localhost", m["database.host"])
	assert.Equal(t, "yaml-secret", m["database.password"])
	assert.Equal(t, "sk-yaml-123", m["api_key"])
	assert.Equal(t, "5432", m["port"])
}

func TestParseFile_UnsupportedFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "readme.md")
	os.WriteFile(path, []byte("# Hello"), 0600)

	_, err := ParseFile(path)
	assert.Error(t, err)
}

func TestMaskValue(t *testing.T) {
	assert.Equal(t, "(empty)", MaskValue(""))
	assert.Equal(t, "****", MaskValue("short"))
	assert.Equal(t, "sk-p...123", MaskValue("sk-proj-abc123"))
	assert.Equal(t, "supe...ret", MaskValue("super-secret"))
}

func fieldMap(fields []ParsedField) map[string]string {
	m := make(map[string]string)
	for _, f := range fields {
		m[f.Key] = f.Value
	}
	return m
}
