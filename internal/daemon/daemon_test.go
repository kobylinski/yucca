package daemon

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"

	"github.com/kobylinski/yucca/internal/store"
)

func testDaemon(t *testing.T) *Daemon {
	t.Helper()
	keyring.MockInit()
	dir := t.TempDir()
	d, err := New(Config{StoreDir: dir, Port: 0})
	require.NoError(t, err)
	return d
}

func TestAPI_Health(t *testing.T) {
	d := testDaemon(t)
	mux := http.NewServeMux()
	d.registerAPI(mux)

	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAPI_CreateAndApproveRequest(t *testing.T) {
	d := testDaemon(t)
	mux := http.NewServeMux()
	d.registerAPI(mux)

	body, _ := json.Marshal(CreateRequestPayload{
		Alias:       "API_KEY",
		Reason:      "testing",
		ProjectPath: "/test/project",
	})
	req := httptest.NewRequest("POST", "/api/requests", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	assert.Equal(t, http.StatusCreated, w.Code)

	var created SecretRequest
	require.NoError(t, json.NewDecoder(w.Body).Decode(&created))
	assert.Equal(t, StatusPending, created.Status)

	approveBody, _ := json.Marshal(ApprovePayload{
		Value:  "my-secret",
		Policy: store.PolicyAskSession,
	})
	req2 := httptest.NewRequest("POST", "/api/requests/"+created.ID+"/approve", bytes.NewReader(approveBody))
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)

	val, meta, err := d.Store.GetCredential("/test/project", "API_KEY")
	require.NoError(t, err)
	assert.Equal(t, "my-secret", val)
	assert.Equal(t, store.PolicyAskSession, meta.Policy)
}

func TestAPI_DenyRequest(t *testing.T) {
	d := testDaemon(t)
	mux := http.NewServeMux()
	d.registerAPI(mux)

	body, _ := json.Marshal(CreateRequestPayload{
		Alias:       "TOKEN",
		Reason:      "deploy",
		ProjectPath: "/test/proj",
	})
	req := httptest.NewRequest("POST", "/api/requests", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var created SecretRequest
	require.NoError(t, json.NewDecoder(w.Body).Decode(&created))

	req2 := httptest.NewRequest("POST", "/api/requests/"+created.ID+"/deny", nil)
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)

	got, _ := d.Queue.Get(created.ID)
	assert.Equal(t, StatusDenied, got.Status)
}

func TestAPI_UpdateCredentialPolicy(t *testing.T) {
	d := testDaemon(t)
	mux := http.NewServeMux()
	d.registerAPI(mux)

	// Create credential via approve flow
	body, _ := json.Marshal(CreateRequestPayload{
		Alias: "KEY", Reason: "test", ProjectPath: "/test/proj",
	})
	req := httptest.NewRequest("POST", "/api/requests", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	var created SecretRequest
	require.NoError(t, json.NewDecoder(w.Body).Decode(&created))

	approveBody, _ := json.Marshal(ApprovePayload{Value: "secret", Policy: store.PolicyAskSession})
	req2 := httptest.NewRequest("POST", "/api/requests/"+created.ID+"/approve", bytes.NewReader(approveBody))
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)

	// Get project slug
	req3 := httptest.NewRequest("GET", "/api/projects", nil)
	w3 := httptest.NewRecorder()
	mux.ServeHTTP(w3, req3)
	var projects []store.ProjectInfo
	require.NoError(t, json.NewDecoder(w3.Body).Decode(&projects))
	slug := projects[0].Slug

	// Update policy
	updateBody, _ := json.Marshal(UpdateCredentialPayload{Policy: store.PolicyAlwaysAllow})
	req4 := httptest.NewRequest("PUT", "/api/projects/"+slug+"/credentials/KEY", bytes.NewReader(updateBody))
	w4 := httptest.NewRecorder()
	mux.ServeHTTP(w4, req4)
	assert.Equal(t, http.StatusOK, w4.Code)

	// Verify policy changed
	_, meta, err := d.Store.GetCredential("/test/proj", "KEY")
	require.NoError(t, err)
	assert.Equal(t, store.PolicyAlwaysAllow, meta.Policy)
}

func TestAPI_DeleteCredentialEndpoint(t *testing.T) {
	d := testDaemon(t)
	mux := http.NewServeMux()
	d.registerAPI(mux)

	// Create credential
	body, _ := json.Marshal(CreateRequestPayload{
		Alias: "KEY", Reason: "test", ProjectPath: "/test/proj",
	})
	req := httptest.NewRequest("POST", "/api/requests", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	var created SecretRequest
	require.NoError(t, json.NewDecoder(w.Body).Decode(&created))

	approveBody, _ := json.Marshal(ApprovePayload{Value: "secret", Policy: store.PolicyAskSession})
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/requests/"+created.ID+"/approve", bytes.NewReader(approveBody)))

	// Get slug
	w3 := httptest.NewRecorder()
	mux.ServeHTTP(w3, httptest.NewRequest("GET", "/api/projects", nil))
	var projects []store.ProjectInfo
	require.NoError(t, json.NewDecoder(w3.Body).Decode(&projects))
	slug := projects[0].Slug

	// Delete
	req4 := httptest.NewRequest("DELETE", "/api/projects/"+slug+"/credentials/KEY", nil)
	w4 := httptest.NewRecorder()
	mux.ServeHTTP(w4, req4)
	assert.Equal(t, http.StatusOK, w4.Code)

	// Verify deleted
	_, _, err := d.Store.GetCredential("/test/proj", "KEY")
	assert.Error(t, err)
}

func TestAPI_SearchCredentials(t *testing.T) {
	d := testDaemon(t)
	mux := http.NewServeMux()
	d.registerAPI(mux)

	projectPath := "/tmp/searchapi"
	d.Store.SetCredential(projectPath, "MY_API_KEY", "secret", store.PolicyAlwaysAllow)
	d.Store.SetCredentialContext(projectPath, "MY_API_KEY", "Stripe production key")

	slug := d.Store.ProjectSlug(projectPath)

	req := httptest.NewRequest("GET", "/api/projects/"+slug+"/credentials/search?q=stripe", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var results []store.CredentialMeta
	json.NewDecoder(w.Body).Decode(&results)
	assert.Len(t, results, 1)
	assert.Equal(t, "MY_API_KEY", results[0].Alias)
}

func TestAPI_SetCredentialContext(t *testing.T) {
	d := testDaemon(t)
	mux := http.NewServeMux()
	d.registerAPI(mux)

	projectPath := "/tmp/contextapi"
	d.Store.SetCredential(projectPath, "MY_KEY", "val", store.PolicyAlwaysAllow)
	slug := d.Store.ProjectSlug(projectPath)

	body, _ := json.Marshal(SetContextPayload{Context: "Some notes about this key"})
	req := httptest.NewRequest("PUT", "/api/projects/"+slug+"/credentials/MY_KEY/context", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	_, meta, err := d.Store.GetCredential(projectPath, "MY_KEY")
	require.NoError(t, err)
	assert.Equal(t, "Some notes about this key", meta.Context)
}

func TestAPI_GetCredentialValuesResolvesFileSource(t *testing.T) {
	d := testDaemon(t)
	mux := http.NewServeMux()
	d.registerAPI(mux)

	projectDir := t.TempDir()
	os.WriteFile(filepath.Join(projectDir, ".env"), []byte("API_KEY=sk-live-abc123\n"), 0600)

	// File-sourced credential (empty value in keychain)
	d.Store.SetCredentialWithSource(projectDir, ".env:API_KEY", "", store.PolicyAlwaysAllow, store.CredentialSource{
		Type:     "file",
		FilePath: ".env",
		FileKey:  "API_KEY",
	})
	// Manual credential
	d.Store.SetCredential(projectDir, "MANUAL_SECRET", "manual-value", store.PolicyAlwaysAllow)

	slug := d.Store.ProjectSlug(projectDir)
	req := httptest.NewRequest("GET", "/api/projects/"+slug+"/credentials/values", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var values map[string]string
	json.NewDecoder(w.Body).Decode(&values)
	assert.Equal(t, "sk-live-abc123", values[".env:API_KEY"])
	assert.Equal(t, "manual-value", values["MANUAL_SECRET"])
}

func TestReplaceInShellContext(t *testing.T) {
	const placeholder = "{{YUCCA:KEY}}"
	const envName = "_YUCCA_TEST"

	tests := []struct {
		name string
		cmd  string
		want string
	}{
		{
			name: "unquoted",
			cmd:  `echo {{YUCCA:KEY}}`,
			want: `echo "$_YUCCA_TEST"`,
		},
		{
			name: "double-quoted",
			cmd:  `curl -H "AccessKey: {{YUCCA:KEY}}"`,
			want: `curl -H "AccessKey: $_YUCCA_TEST"`,
		},
		{
			name: "single-quoted header",
			// 'AccessKey: '"$ENVNAME"'' — trailing '' is empty string, harmless
			cmd:  `curl -H 'AccessKey: {{YUCCA:KEY}}'`,
			want: `curl -H 'AccessKey: '"$_YUCCA_TEST"''`,
		},
		{
			name: "single-quoted printf pattern",
			// printf receives ''"$ENVNAME"'' which concatenates to just the value
			cmd:  `KEY=$(printf '%s' '{{YUCCA:KEY}}' | tr -d '\n'); echo $KEY`,
			want: `KEY=$(printf '%s' ''"$_YUCCA_TEST"'' | tr -d '\n'); echo $KEY`,
		},
		{
			name: "no placeholder",
			cmd:  `echo hello`,
			want: `echo hello`,
		},
		{
			name: "multiple occurrences mixed quoting",
			cmd:  `echo {{YUCCA:KEY}} 'val={{YUCCA:KEY}}'`,
			want: `echo "$_YUCCA_TEST" 'val='"$_YUCCA_TEST"''`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := replaceInShellContext(tt.cmd, placeholder, envName)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSessionApprovals(t *testing.T) {
	a := NewSessionApprovals()
	assert.False(t, a.Approved("proj", "KEY"))
	a.Approve("proj", "KEY")
	assert.True(t, a.Approved("proj", "KEY"), "approved this session")
	assert.False(t, a.Approved("proj", "OTHER"), "other alias still prompts")
	assert.False(t, a.Approved("other", "KEY"), "other project still prompts")
	a.ClearProject("proj")
	assert.False(t, a.Approved("proj", "KEY"), "cleared when session ends")
}

func TestSessionApproval_RememberedOnApprove_ClearedOnRegister(t *testing.T) {
	d := testDaemon(t)
	mux := http.NewServeMux()
	d.registerAPI(mux)

	projectPath := "/tmp/sessapprov"
	d.Store.SetCredential(projectPath, "DEPLOY_KEY", "secret", store.PolicyAskSession)
	slug := d.Store.ProjectSlug(projectPath)

	d.Queue.Add(&SecretRequest{
		ID: "req1", Kind: KindExecuteAccept, Aliases: []string{"DEPLOY_KEY"},
		ProjectPath: projectPath, ProjectSlug: slug, Status: StatusPending,
	})
	assert.False(t, d.Approvals.Approved(slug, "DEPLOY_KEY"), "not approved before approval")

	body, _ := json.Marshal(ApprovePayload{Policy: store.PolicyAskSession})
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("POST", "/api/requests/req1/approve", bytes.NewReader(body)))
	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, d.Approvals.Approved(slug, "DEPLOY_KEY"), "remembered after approval — second exec skips prompt")

	regBody, _ := json.Marshal(SessionPayload{ProjectSlug: slug, ProjectPath: projectPath})
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, httptest.NewRequest("POST", "/api/sessions/register", bytes.NewReader(regBody)))
	assert.False(t, d.Approvals.Approved(slug, "DEPLOY_KEY"), "cleared when a new session registers")
}

func TestLocalhostGuard(t *testing.T) {
	const port = 9777
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	guard := localhostGuard(ok, port)

	tests := []struct {
		name   string
		host   string
		origin string
		want   int
	}{
		{"loopback host, no origin", "127.0.0.1:9777", "", http.StatusOK},
		{"localhost host", "localhost:9777", "", http.StatusOK},
		{"loopback origin", "127.0.0.1:9777", "http://127.0.0.1:9777", http.StatusOK},
		{"foreign host (DNS rebind)", "evil.com", "", http.StatusForbidden},
		{"loopback host, wrong port", "127.0.0.1:1234", "", http.StatusForbidden},
		{"cross-origin (CSRF)", "127.0.0.1:9777", "https://evil.com", http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://x/api/health", nil)
			req.Host = tt.host
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			rec := httptest.NewRecorder()
			guard.ServeHTTP(rec, req)
			assert.Equal(t, tt.want, rec.Code)
		})
	}

	// Anti-framing headers are always set on allowed requests.
	req := httptest.NewRequest(http.MethodGet, "http://x/", nil)
	req.Host = "127.0.0.1:9777"
	rec := httptest.NewRecorder()
	guard.ServeHTTP(rec, req)
	assert.Equal(t, "DENY", rec.Header().Get("X-Frame-Options"))
	assert.Contains(t, rec.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'")
}

func TestAPI_SyncCredentials(t *testing.T) {
	d := testDaemon(t)
	mux := http.NewServeMux()
	d.registerAPI(mux)

	projectDir := t.TempDir()
	os.WriteFile(filepath.Join(projectDir, ".env"), []byte("API_KEY=test123\n"), 0600)

	slug := d.Store.ProjectSlug(projectDir)

	payload := SyncPayload{
		ProjectPath: projectDir,
		ProjectName: "myproject",
		Secrets: []SyncSecret{
			{File: ".env", Key: "API_KEY"},
			{File: ".env", Key: "DB_PASS"},
		},
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/api/projects/"+slug+"/sync", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	creds, _ := d.Store.ListCredentials(projectDir)
	assert.Contains(t, creds, ".env:API_KEY")
	assert.Equal(t, "file", creds[".env:API_KEY"].Source.Type)
	assert.Equal(t, ".env", creds[".env:API_KEY"].Source.FilePath)
	assert.Equal(t, "API_KEY", creds[".env:API_KEY"].Source.FileKey)
}
