package exec

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"

	"github.com/kobylinski/yucca/internal/store"
)

var validEnvVarName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// MaskSecrets replaces all occurrences of secret values in output with "***".
// Longer values are replaced first to handle overlapping substrings correctly.
func MaskSecrets(output string, secrets map[string]string) string {
	// Sort by value length descending so longer secrets are masked first
	type kv struct{ k, v string }
	sorted := make([]kv, 0, len(secrets))
	for k, v := range secrets {
		if v != "" {
			sorted = append(sorted, kv{k, v})
		}
	}
	sort.Slice(sorted, func(i, j int) bool {
		return len(sorted[i].v) > len(sorted[j].v)
	})
	for _, s := range sorted {
		output = strings.ReplaceAll(output, s.v, "***")
	}
	return output
}

// Run executes a command with secrets from the store injected as environment
// variables. Used by the CLI `yucca exec` command.
func Run(s *store.Store, projectPath string, command []string) (int, error) {
	if len(command) == 0 {
		return 1, fmt.Errorf("no command specified")
	}

	creds, err := s.ListCredentials(projectPath)
	if err != nil {
		return 1, fmt.Errorf("list credentials: %w", err)
	}

	secrets := make(map[string]string)
	for alias := range creds {
		val, _, err := s.GetCredential(projectPath, alias)
		if err != nil {
			continue
		}
		secrets[alias] = val
	}

	stdout, stderr, exitCode, runErr := RunCapture(secrets, projectPath, command)

	fmt.Fprint(os.Stdout, stdout)
	fmt.Fprint(os.Stderr, stderr)

	return exitCode, runErr
}

// RunCapture executes a command with pre-resolved secrets injected as
// environment variables. Returns masked stdout/stderr.
func RunCapture(secrets map[string]string, projectPath string, command []string) (stdout, stderr string, exitCode int, err error) {
	env := os.Environ()
	for alias, val := range secrets {
		if !validEnvVarName.MatchString(alias) {
			continue
		}
		env = append(env, fmt.Sprintf("%s=%s", alias, val))
	}
	return RunCaptureWithEnv(secrets, projectPath, command, env)
}

// RunCaptureWithEnv executes a command with a custom environment.
// Secret values in output are masked. Used by the daemon exec endpoint
// where secrets are mapped to random env var names.
func RunCaptureWithEnv(secrets map[string]string, projectPath string, command, env []string) (stdout, stderr string, exitCode int, err error) {
	if len(command) == 0 {
		return "", "", 1, fmt.Errorf("no command specified")
	}

	cmd := exec.Command(command[0], command[1:]...)
	cmd.Env = env
	cmd.Dir = projectPath

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	runErr := cmd.Run()

	maskedOut := MaskSecrets(outBuf.String(), secrets)
	maskedErr := MaskSecrets(errBuf.String(), secrets)

	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			return maskedOut, maskedErr, exitErr.ExitCode(), nil
		}
		return maskedOut, maskedErr, 1, runErr
	}
	return maskedOut, maskedErr, 0, nil
}
