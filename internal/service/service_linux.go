//go:build linux

package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const unitName = "yucca-daemon.service"

func descriptorPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "systemd", "user", unitName)
}

func render(binPath string) string {
	return fmt.Sprintf(`[Unit]
Description=Yucca daemon

[Service]
ExecStart=%s daemon --idle-timeout 0
Restart=always
RestartSec=2

[Install]
WantedBy=default.target
`, binPath)
}

// WriteDescriptor writes the systemd --user unit pointing at binPath.
func WriteDescriptor(binPath string) (bool, error) {
	if err := os.MkdirAll(filepath.Dir(descriptorPath()), 0755); err != nil {
		return false, err
	}
	return writeIfChanged(descriptorPath(), render(binPath), 0644)
}

// Start enables+starts the unit. Pass reload=true when the unit file changed
// so systemd re-reads it and the running daemon is restarted.
func Start(reload bool) error {
	_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
	if err := exec.Command("systemctl", "--user", "enable", "--now", unitName).Run(); err != nil {
		return fmt.Errorf("systemctl --user enable --now: %w", err)
	}
	if reload {
		_ = exec.Command("systemctl", "--user", "restart", unitName).Run()
	}
	return nil
}

// Stop disables and stops the unit.
func Stop() error {
	return exec.Command("systemctl", "--user", "disable", "--now", unitName).Run()
}

// Remove stops the unit and deletes the unit file.
func Remove() error {
	_ = Stop()
	if err := os.Remove(descriptorPath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
	return nil
}

// Available reports whether a systemd --user instance is reachable.
func Available() bool {
	return exec.Command("systemctl", "--user", "--version").Run() == nil
}
