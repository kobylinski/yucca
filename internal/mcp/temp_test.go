package mcp

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kobylinski/yucca/internal/daemon"
	"github.com/kobylinski/yucca/internal/store"
	"github.com/zalando/go-keyring"
)

func TestTempStoreUnit(t *testing.T) {
	ts := newTempStore()
	ts.Put("TOK", "abc123", true)
	ts.Put("reminder", "check the logs", false)

	if v := ts.SecretValues(); v["TOK"] != "abc123" || len(v) != 1 {
		t.Fatalf("SecretValues = %v", v)
	}
	if r := ts.SecretsReferencedIn("curl -H {{YUCCA:TOK}}"); r["TOK"] != "abc123" {
		t.Fatalf("SecretsReferencedIn missed TOK: %v", r)
	}
	if r := ts.SecretsReferencedIn("no placeholder"); len(r) != 0 {
		t.Fatalf("SecretsReferencedIn should be empty: %v", r)
	}
	if a := ts.SecretAliases(); len(a) != 1 || a[0] != "TOK" {
		t.Fatalf("SecretAliases = %v", a)
	}
	notes := ts.Notes()
	if len(notes) != 1 || notes[0].Alias != "reminder" || notes[0].Body != "check the logs" {
		t.Fatalf("Notes = %v", notes)
	}

	// Server helpers
	srv := New("", "/tmp/x", nil)
	srv.temp.Put("TMP", "s3cr3t", true)
	merged := srv.withTempRedaction(map[string]string{"REAL": "rv"})
	if merged["TMP"] != "s3cr3t" || merged["REAL"] != "rv" {
		t.Fatalf("withTempRedaction = %v", merged)
	}
	if srv.tempPlaceholderIn("x={{YUCCA:TMP}}") != "TMP" {
		t.Fatal("tempPlaceholderIn should detect TMP")
	}
	if srv.tempPlaceholderIn("nothing") != "" {
		t.Fatal("tempPlaceholderIn false positive")
	}
}

// TestTempSecretExecAndCollision drives the full temp-secret path through a real
// httptest daemon (no SSH): store a temp secret → use it in exec → confirm the
// value is injected but masked in output (never leaked) → confirm a temp alias
// can't shadow a persisted credential.
func TestTempSecretExecAndCollision(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()
	d, err := daemon.New(daemon.Config{StoreDir: dir, Port: 0, IdleTimeout: 0})
	if err != nil {
		t.Fatalf("daemon.New: %v", err)
	}
	project := t.TempDir() // a real dir — exec runs with cmd.Dir = projectPath
	slug := strings.ReplaceAll(project, "/", "-")
	d.Sessions.Register(slug, project, "temp-exec-test") // resolves via the session fallback

	ts := httptest.NewServer(d.Handler())
	defer ts.Close()
	srv := New(ts.URL, project, nil)

	no := false
	sa, _ := json.Marshal(secretStoreArgs{Alias: "TMP_TOKEN", Value: "tok-abc-123", Persist: &no})
	if text, isErr := toolText(srv.handleSecretStore(1, sa)); isErr {
		t.Fatalf("temp store failed: %s", text)
	}

	ea, _ := json.Marshal(execArgs{Command: "echo USING={{YUCCA:TMP_TOKEN}}"})
	text, isErr := toolText(srv.handleExec(2, ea))
	if isErr {
		t.Fatalf("exec failed: %s", text)
	}
	if strings.Contains(text, "tok-abc-123") {
		t.Fatalf("exec leaked the temp secret value: %s", text)
	}
	if !strings.Contains(text, "USING=") || !strings.Contains(text, "***") {
		t.Fatalf("expected masked 'USING=***', got: %s", text)
	}

	// Collision: a temp alias must not shadow a persisted credential.
	if err := d.Store.SetCredential(project, "REALSECRET", "v", store.PolicyAlwaysAllow); err != nil {
		t.Fatalf("set persisted: %v", err)
	}
	ca, _ := json.Marshal(secretStoreArgs{Alias: "REALSECRET", Value: "x", Persist: &no})
	if _, isErr := toolText(srv.handleSecretStore(3, ca)); !isErr {
		t.Fatal("temp store should refuse an alias that collides with a persisted secret")
	}
}
