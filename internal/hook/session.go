package hook

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"
)

// HandleSessionStart ensures the daemon is running and exports env vars
func HandleSessionStart(input *HookInput, daemonAddr string) error {
	// Check if daemon is already running
	if !isDaemonRunning(daemonAddr) {
		// Start daemon in background
		cmd := exec.Command(os.Args[0], "daemon")
		cmd.Stdout = nil
		cmd.Stderr = nil
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("start daemon: %w", err)
		}
		// Release the process so it keeps running
		if err := cmd.Process.Release(); err != nil {
			return fmt.Errorf("release daemon process: %w", err)
		}
		// Wait for daemon to be ready
		started := false
		for i := 0; i < 20; i++ {
			time.Sleep(250 * time.Millisecond)
			if isDaemonRunning(daemonAddr) {
				started = true
				break
			}
		}
		if !started {
			return fmt.Errorf("daemon failed to start at %s after 5s", daemonAddr)
		}
	}

	// Write env vars to CLAUDE_ENV_FILE
	envFile := os.Getenv("CLAUDE_ENV_FILE")
	if envFile != "" {
		f, err := os.OpenFile(envFile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
		if err != nil {
			return fmt.Errorf("open env file: %w", err)
		}
		defer f.Close()
		fmt.Fprintf(f, "export YUCCA_SESSION_ID='%s'\n", input.SessionID)
		fmt.Fprintf(f, "export YUCCA_DAEMON='%s'\n", daemonAddr)
		fmt.Fprintf(f, "export YUCCA_PROJECT='%s'\n", input.Cwd)
	}

	// Print context for Claude
	fmt.Print(SessionStartOutput(input.SessionID, daemonAddr))
	return nil
}

// HandleSessionEnd cleans up session state
func HandleSessionEnd(input *HookInput) error {
	// Currently a no-op — secrets persist in keychain across sessions.
	// Session-scoped grants (ask_session) could be tracked here in future.
	return nil
}

func isDaemonRunning(addr string) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(addr + "/api/health")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
