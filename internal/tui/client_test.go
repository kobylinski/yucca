package tui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_Health(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/health", r.URL.Path)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	err := c.Health()
	require.NoError(t, err)
}

func TestClient_Health_Unreachable(t *testing.T) {
	c := NewClient("http://127.0.0.1:1")
	err := c.Health()
	assert.Error(t, err)
}

func TestClient_FetchProjects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]ProjectInfo{
			{Path: "/home/dev/myapp", Name: "myapp", Slug: "abc123"},
		})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	projects, err := c.FetchProjects()
	require.NoError(t, err)
	assert.Len(t, projects, 1)
	assert.Equal(t, "myapp", projects[0].Name)
}

func TestClient_FetchCredentials(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]CredentialMeta{
			"API_KEY": {Alias: "API_KEY", Policy: "ask_session"},
		})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	creds, err := c.FetchCredentials("abc123")
	require.NoError(t, err)
	assert.Len(t, creds, 1)
	assert.Equal(t, "API_KEY", creds["API_KEY"].Alias)
}

func TestClient_FetchPendingRequests(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]SecretRequest{
			{ID: "req1", Alias: "TOKEN", Status: "pending"},
		})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	reqs, err := c.FetchPendingRequests()
	require.NoError(t, err)
	assert.Len(t, reqs, 1)
	assert.Equal(t, "TOKEN", reqs[0].Alias)
}

func TestClient_ApproveRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/requests/req1/approve", r.URL.Path)
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		assert.Equal(t, "s3cret", body["value"])
		assert.Equal(t, "ask_session", body["policy"])
		json.NewEncoder(w).Encode(map[string]string{"status": "approved"})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	err := c.ApproveRequest("req1", "s3cret", "ask_session")
	require.NoError(t, err)
}

func TestClient_CreateCredential(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/projects/abc123/credentials", r.URL.Path)
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		assert.Equal(t, "API_KEY", body["alias"])
		assert.Equal(t, "s3cret", body["value"])
		assert.Equal(t, "ask_session", body["policy"])
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"status": "created"})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	err := c.CreateCredential("abc123", "API_KEY", "s3cret", "ask_session")
	require.NoError(t, err)
}

func TestClient_CreateCredential_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{"error": "alias already exists"})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	err := c.CreateCredential("abc123", "API_KEY", "s3cret", "ask_session")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "alias already exists")
}

func TestClient_SetCredentialContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "PUT", r.Method)
		assert.Equal(t, "/api/projects/abc123/credentials/API_KEY/context", r.URL.Path)
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		assert.Equal(t, "Used for external API calls", body["context"])
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	err := c.SetCredentialContext("abc123", "API_KEY", "Used for external API calls")
	require.NoError(t, err)
}

func TestClient_FetchSessions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/sessions", r.URL.Path)
		json.NewEncoder(w).Encode([]ActiveSession{
			{ProjectSlug: "abc123", ProjectPath: "/home/dev/myapp", ProjectName: "myapp"},
			{ProjectSlug: "def456", ProjectPath: "/home/dev/other", ProjectName: "other"},
		})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	sessions, err := c.FetchSessions()
	require.NoError(t, err)
	assert.Len(t, sessions, 2)
	assert.Equal(t, "myapp", sessions[0].ProjectName)
	assert.Equal(t, "other", sessions[1].ProjectName)
}

func TestClient_FetchSessions_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]ActiveSession{})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	sessions, err := c.FetchSessions()
	require.NoError(t, err)
	assert.Empty(t, sessions)
}

func TestClient_DenyRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/requests/req1/deny", r.URL.Path)
		json.NewEncoder(w).Encode(map[string]string{"status": "denied"})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	err := c.DenyRequest("req1")
	require.NoError(t, err)
}
