package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kobylinski/yucca/internal/scanner"
)

func TestRedactUnregisteredValues(t *testing.T) {
	// API_KEY is already redacted (registered); DB_PASSWORD is an unregistered
	// secret in the same protected file and must not pass through verbatim.
	content := "API_KEY={{YUCCA:.env:API_KEY}}\nDB_PASSWORD=sup3rs3cretvalue\n"
	fields := []scanner.ParsedField{
		{Key: "DB_PASSWORD", Value: "sup3rs3cretvalue"},
	}
	out := redactUnregisteredValues(content, fields)
	assert.NotContains(t, out, "sup3rs3cretvalue", "unregistered secret redacted")
	assert.Contains(t, out, "{{YUCCA:DB_PASSWORD}}")
	assert.Contains(t, out, "{{YUCCA:.env:API_KEY}}", "registered placeholder left intact")
}

func TestHandleInitialize(t *testing.T) {
	s := New("http://127.0.0.1:9777", "/tmp/test", nil)

	req := jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      float64(1),
		Method:  "initialize",
	}

	resp := s.handleRequest(req)
	assert.Equal(t, "2.0", resp.JSONRPC)
	assert.Equal(t, float64(1), resp.ID)

	result, ok := resp.Result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "2024-11-05", result["protocolVersion"])

	info := result["serverInfo"].(map[string]string)
	assert.Equal(t, "yucca", info["name"])
}

func TestHandleToolsList(t *testing.T) {
	s := New("http://127.0.0.1:9777", "/tmp/test", nil)

	req := jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      float64(2),
		Method:  "tools/list",
	}

	resp := s.handleRequest(req)
	result, ok := resp.Result.(map[string]any)
	require.True(t, ok)

	tools := result["tools"].([]map[string]any)
	assert.Len(t, tools, 11)

	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t["name"].(string)
	}
	assert.Contains(t, names, "yucca_secret_request")
	assert.Contains(t, names, "yucca_file")
	assert.Contains(t, names, "yucca_exec")
	assert.Contains(t, names, "yucca_clipboard")
	assert.Contains(t, names, "yucca_secret_index")
	assert.Contains(t, names, "yucca_secret_context")
	assert.Contains(t, names, "yucca_secret_store")
	assert.Contains(t, names, "yucca_note_store")
	assert.Contains(t, names, "yucca_note_list")
}

func TestHandleUnknownMethod(t *testing.T) {
	s := New("http://127.0.0.1:9777", "/tmp/test", nil)

	req := jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      float64(3),
		Method:  "unknown/method",
	}

	resp := s.handleRequest(req)
	assert.NotNil(t, resp.Error)

	errMap := resp.Error.(map[string]any)
	assert.Equal(t, -32601, errMap["code"])
}

func TestHandleToolCallUnknownTool(t *testing.T) {
	s := New("http://127.0.0.1:9777", "/tmp/test", nil)

	params, _ := json.Marshal(toolCallParams{
		Name:      "unknown_tool",
		Arguments: json.RawMessage(`{}`),
	})

	req := jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      float64(4),
		Method:  "tools/call",
		Params:  params,
	}

	resp := s.handleRequest(req)
	assert.NotNil(t, resp.Error)
}

func TestSyncOnStartup(t *testing.T) {
	projectDir := t.TempDir()
	yuccaYAML := "project_name: testproject\nprotected_files:\n  - .env\n\nsecrets:\n  - file: .env\n    key: API_KEY\n  - file: .env\n    key: DB_PASS\n"
	os.WriteFile(filepath.Join(projectDir, ".yucca.yaml"), []byte(yuccaYAML), 0600)

	var syncReceived syncPayload
	var syncCalled bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/sync") {
			syncCalled = true
			json.NewDecoder(r.Body).Decode(&syncReceived)
			json.NewEncoder(w).Encode(map[string]any{"synced": 2})
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer ts.Close()

	srv := New(ts.URL, projectDir, nil)
	srv.syncCredentials()

	assert.True(t, syncCalled)
	assert.Equal(t, projectDir, syncReceived.ProjectPath)
	assert.Len(t, syncReceived.Secrets, 2)
	assert.Equal(t, ".env", syncReceived.Secrets[0].File)
	assert.Equal(t, "API_KEY", syncReceived.Secrets[0].Key)
	assert.Equal(t, "DB_PASS", syncReceived.Secrets[1].Key)
}
