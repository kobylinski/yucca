package mcp

import (
	"os"
	"testing"
)

// TestRemoteSSHLive exercises the real sshWrite/sshRead against a live host.
// Set YUCCA_SSH_TEST_HOST=user@host to run; otherwise it is skipped, so it
// never runs in CI or a normal `go test ./...`.
func TestRemoteSSHLive(t *testing.T) {
	host := os.Getenv("YUCCA_SSH_TEST_HOST")
	if host == "" {
		t.Skip("set YUCCA_SSH_TEST_HOST=user@host to run the live SSH test")
	}
	// ~/ path forces the home-expansion + quoting path in remoteShellPath.
	path := "~/.yucca-remote-test.env"
	// Realistic content: multiline, an unregistered-looking secret, spaces,
	// single quotes, and a $dollar — all of which must survive verbatim.
	payload := "# yucca remote-file test\n" +
		"API_KEY=sk-live-ABC123def456\n" +
		"DB_URL=postgres://user:p@ss@db:5432/app\n" +
		"NOTE=has spaces, 'single quotes', and $DOLLAR\n" +
		"LAST=ok\n"

	if err := sshWrite(host, path, payload); err != nil {
		t.Fatalf("sshWrite: %v", err)
	}
	got, err := sshRead(host, path)
	if err != nil {
		t.Fatalf("sshRead: %v", err)
	}
	if got != payload {
		t.Fatalf("round-trip mismatch:\n--- want (%d bytes) ---\n%q\n--- got (%d bytes) ---\n%q",
			len(payload), payload, len(got), got)
	}
	t.Logf("round-trip OK: %d bytes byte-exact via SSH to %s:%s", len(got), host, path)
}
