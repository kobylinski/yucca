package mcp

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/kobylinski/yucca/internal/daemon"
	"github.com/kobylinski/yucca/internal/store"
	"github.com/zalando/go-keyring"
)

// toolText extracts the text payload and error flag from a tool response.
func toolText(resp jsonrpcResponse) (string, bool) {
	m, _ := resp.Result.(map[string]any)
	if m == nil {
		return "", true
	}
	isErr, _ := m["isError"].(bool)
	content, _ := m["content"].([]map[string]any)
	if len(content) == 0 {
		return "", isErr
	}
	text, _ := content[0]["text"].(string)
	return text, isErr
}

// TestRemoteFileE2E exercises the FULL daemon-mediated remote path: a secret
// stored in the daemon's store, referenced by {{TRUSTEE:alias}}, rehydrated to
// its real value and written to a remote host over SSH — then read back through
// the tool to confirm the model only ever sees the redacted placeholder.
// Keychain is mocked (no Touch ID / no pollution); the daemon runs via httptest
// (no real port / lockfile). Gated on YUCCA_SSH_TEST_HOST=user@host.
func TestRemoteFileE2E(t *testing.T) {
	host := os.Getenv("YUCCA_SSH_TEST_HOST")
	if host == "" {
		t.Skip("set YUCCA_SSH_TEST_HOST=user@host to run the live e2e")
	}
	keyring.MockInit()

	dir := t.TempDir()
	d, err := daemon.New(daemon.Config{StoreDir: dir, Port: 0, IdleTimeout: 0})
	if err != nil {
		t.Fatalf("daemon.New: %v", err)
	}
	const project = "/tmp/yucca-e2e-project"
	const alias = "DB_PASSWORD"
	const secret = "s3cr3t-live-value-9f8e7d"
	if err := d.Store.SetCredential(project, alias, secret, store.PolicyAlwaysAllow); err != nil {
		t.Fatalf("store secret: %v", err)
	}

	ts := httptest.NewServer(d.Handler())
	defer ts.Close()

	srv := New(ts.URL, project, d.Store)

	const remoteFile = "~/.yucca-e2e-test.env"
	remotePath := host + ":" + remoteFile
	defer exec.Command("ssh", append(append([]string{}, sshOpts...), host, "rm -f "+remoteShellPath(remoteFile))...).Run()

	content := "DB_URL=postgres://app:{{TRUSTEE:DB_PASSWORD}}@db:5432/app\nTOKEN={{TRUSTEE:DB_PASSWORD}}\n"

	// WRITE: placeholders rehydrated to the real value and sent over SSH.
	writeArgs, _ := json.Marshal(fileArgs{Action: "write", FilePath: remotePath, Content: content})
	if text, isErr := toolText(srv.handleFile(1, writeArgs)); isErr {
		t.Fatalf("handleFile write failed: %s", text)
	}

	// Independently confirm what actually landed on the server.
	onDisk, err := sshRead(host, remoteFile)
	if err != nil {
		t.Fatalf("sshRead verify: %v", err)
	}
	if !strings.Contains(onDisk, secret) {
		t.Fatal("remote file does NOT contain the real secret value")
	}
	if strings.Contains(onDisk, "{{TRUSTEE:") {
		t.Fatal("remote file still contains an unrehydrated placeholder")
	}
	t.Log("WRITE ok: secret rehydrated to its real value on the server, no placeholder left")

	// READ: the real value on the server must come back redacted for the model.
	readArgs, _ := json.Marshal(fileArgs{Action: "read", FilePath: remotePath})
	got, isErr := toolText(srv.handleFile(2, readArgs))
	if isErr {
		t.Fatalf("handleFile read failed: %s", got)
	}
	if strings.Contains(got, secret) {
		t.Fatal("READ leaked the raw secret to the model (should be redacted)")
	}
	if !strings.Contains(got, "{{TRUSTEE:DB_PASSWORD}}") {
		t.Fatal("READ did not redact the secret back to {{TRUSTEE:DB_PASSWORD}}")
	}
	t.Logf("READ ok: model sees only redacted content:\n%s", got)
}
