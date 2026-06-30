package mcp

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/kobylinski/yucca/internal/proxy"
)

// parseRemotePath detects an scp-style remote path "[user@]host:path".
// It returns the ssh host spec (including any "user@") and the remote path.
// A local path (no colon, or a colon only after a slash like /a:b) returns remote=false.
func parseRemotePath(p string) (host, path string, remote bool) {
	i := strings.IndexByte(p, ':')
	if i <= 0 {
		return "", "", false
	}
	hostPart := p[:i]
	// A separator before the colon means it's a local path, not host:path.
	if strings.ContainsAny(hostPart, `/\`) {
		return "", "", false
	}
	pathPart := p[i+1:]
	if hostPart == "" || pathPart == "" {
		return "", "", false
	}
	return hostPart, pathPart, true
}

// remoteShellPath quotes a remote path for a remote shell command while
// preserving a leading "~/" (home expansion). Everything after it is
// single-quoted so spaces and shell metacharacters stay inert.
func remoteShellPath(p string) string {
	if p == "~" {
		return "~"
	}
	prefix, rest := "", p
	if strings.HasPrefix(p, "~/") {
		prefix, rest = "~/", p[2:]
	}
	return prefix + "'" + strings.ReplaceAll(rest, "'", `'\''`) + "'"
}

// sshOpts fail fast instead of hanging on prompts (the MCP process has no TTY)
// and time out unreachable hosts. Auth comes from the user's ssh agent/config.
var sshOpts = []string{"-o", "BatchMode=yes", "-o", "ConnectTimeout=10"}

func sshRead(host, path string) (string, error) {
	args := append(append([]string{}, sshOpts...), host, "cat -- "+remoteShellPath(path))
	cmd := exec.Command("ssh", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%v: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

func sshWrite(host, path, content string) error {
	args := append(append([]string{}, sshOpts...), host, "cat > "+remoteShellPath(path))
	cmd := exec.Command("ssh", args...)
	cmd.Stdin = strings.NewReader(content) // secrets stream via stdin, never disk/args
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// handleRemoteFile reads or writes a file on a remote host over SSH, applying the
// same redact-on-read / rehydrate-on-write as local protected files. Secret values
// are resolved in memory and streamed over SSH — they never touch local disk, and
// the rehydrated content lands on the remote host in one atomic write (so a
// server-side secret sanitizer never sees raw values mid-write).
func (s *Server) handleRemoteFile(id any, host, path string, args fileArgs) jsonrpcResponse {
	secrets, err := s.fetchCredentialValues()
	if err != nil {
		return s.toolResult(id, fmt.Sprintf("Error fetching credentials: %v", err), true)
	}

	switch args.Action {
	case "read":
		out, err := sshRead(host, path)
		if err != nil {
			return s.toolResult(id, fmt.Sprintf("Error reading %s:%s over SSH: %v", host, path, err), true)
		}
		redactMap := s.withTempRedaction(secrets)
		if len(redactMap) > 0 {
			out = proxy.Redact(out, redactMap)
		}
		return s.toolResult(id, out, false)

	case "write":
		if args.Content == "" {
			return s.toolResult(id, "Content is required for write action", true)
		}
		if a := s.tempPlaceholderIn(args.Content); a != "" {
			return s.toolResult(id, fmt.Sprintf("Refusing to write: %q is a TEMPORARY secret and must not be persisted to a file (it lives only in this session). Use yucca_secret_store for a value that should be written.", a), true)
		}
		content := args.Content
		if len(secrets) > 0 {
			content = proxy.Sanitize(content, secrets)
			content = proxy.Rehydrate(content, secrets)
		}
		if err := sshWrite(host, path, content); err != nil {
			return s.toolResult(id, fmt.Sprintf("Error writing %s:%s over SSH: %v", host, path, err), true)
		}
		return s.toolResult(id, fmt.Sprintf("File written over SSH: %s:%s", host, path), false)

	default:
		return s.toolResult(id, "Action must be 'read' or 'write'", true)
	}
}
