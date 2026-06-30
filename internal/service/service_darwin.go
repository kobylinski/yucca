//go:build darwin

package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const label = "co.kobylinski.yucca.daemon"

func descriptorPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", label+".plist")
}

func domain() string { return fmt.Sprintf("gui/%d", os.Getuid()) }

func render(binPath, logPath string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>%s</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>daemon</string>
        <string>--idle-timeout</string>
        <string>0</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>ProcessType</key>
    <string>Background</string>
    <key>StandardOutPath</key>
    <string>%s</string>
    <key>StandardErrorPath</key>
    <string>%s</string>
</dict>
</plist>
`, label, binPath, logPath, logPath)
}

// WriteDescriptor writes the LaunchAgent plist pointing at binPath.
func WriteDescriptor(binPath string) (bool, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return false, fmt.Errorf("resolve home dir: %w", err)
	}
	_ = os.MkdirAll(filepath.Join(home, ".yucca"), 0700)
	if err := os.MkdirAll(filepath.Dir(descriptorPath()), 0755); err != nil {
		return false, err
	}
	logPath := filepath.Join(home, ".yucca", "daemon.log")
	return writeIfChanged(descriptorPath(), render(binPath, logPath), 0644)
}

func isLoaded() bool {
	return exec.Command("launchctl", "print", domain()+"/"+label).Run() == nil
}

// Start ensures the LaunchAgent is loaded and the daemon is running. Pass
// reload=true when the plist content changed, so launchd re-reads it (a plain
// kickstart keeps the cached definition). bootout is async, so we wait for the
// service to actually unload before bootstrapping to avoid an EBUSY race.
func Start(reload bool) error {
	svc := domain() + "/" + label
	if reload && isLoaded() {
		_ = exec.Command("launchctl", "bootout", svc).Run()
		for i := 0; i < 40 && isLoaded(); i++ {
			time.Sleep(100 * time.Millisecond)
		}
	}
	if isLoaded() {
		// Already managed with the current definition — restart in place
		// (picks up a rebuilt binary at the same path).
		return exec.Command("launchctl", "kickstart", "-k", svc).Run()
	}
	if err := exec.Command("launchctl", "bootstrap", domain(), descriptorPath()).Run(); err != nil {
		return fmt.Errorf("launchctl bootstrap: %w", err)
	}
	return nil
}

// Stop unloads the LaunchAgent (daemon will not be restarted until reloaded).
func Stop() error {
	return exec.Command("launchctl", "bootout", domain()+"/"+label).Run()
}

// Remove stops the service and deletes the plist.
func Remove() error {
	_ = Stop()
	if err := os.Remove(descriptorPath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Available reports whether launchctl is usable.
func Available() bool {
	return exec.Command("launchctl", "print", domain()).Run() == nil
}
