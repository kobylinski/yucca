// Package clipboard provides write-only access to the OS clipboard for
// injecting secret values that the user needs to paste somewhere Yucca
// cannot reach directly. It is deliberately write-only: there is no public
// API to read the clipboard, so a secret manager never exposes a path to
// exfiltrate whatever the user has copied.
//
// Backends are selected at runtime per OS — no cgo, so the daemon still
// cross-compiles cleanly for macOS and Linux:
//
//	macOS         pbcopy / pbpaste
//	Linux Wayland wl-copy / wl-paste     (package: wl-clipboard)
//	Linux X11     xclip, falling back to xsel
//
// On Linux the relevant helper must be installed and the daemon must inherit
// the graphical session env (WAYLAND_DISPLAY / DISPLAY); otherwise Copy
// returns a descriptive error.
package clipboard

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"time"
)

// Copy writes value to the system clipboard.
func Copy(value string) error {
	name, args, err := copyCommand()
	if err != nil {
		return err
	}
	cmd := exec.Command(name, args...)
	cmd.Stdin = bytes.NewBufferString(value)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

// CopyWithClear writes value to the clipboard and, if after > 0, schedules a
// background clear once the duration elapses. The clear only happens if the
// clipboard still holds value — so a later copy by the user is never wiped
// (the password-manager pattern). The clear is best-effort and silent.
func CopyWithClear(value string, after time.Duration) error {
	if err := Copy(value); err != nil {
		return err
	}
	if after > 0 && value != "" {
		go func() {
			time.Sleep(after)
			if cur, err := read(); err == nil && cur == value {
				_ = Copy("")
			}
		}()
	}
	return nil
}

// copyCommand returns the helper command (and args) that reads the clipboard
// value from stdin for the current OS/session.
func copyCommand() (string, []string, error) {
	switch runtime.GOOS {
	case "darwin":
		return "pbcopy", nil, nil
	case "linux":
		if os.Getenv("WAYLAND_DISPLAY") != "" {
			if p, err := exec.LookPath("wl-copy"); err == nil {
				return p, nil, nil
			}
		}
		if p, err := exec.LookPath("xclip"); err == nil {
			return p, []string{"-selection", "clipboard"}, nil
		}
		if p, err := exec.LookPath("xsel"); err == nil {
			return p, []string{"--clipboard", "--input"}, nil
		}
		return "", nil, fmt.Errorf("no clipboard helper found: install wl-clipboard (Wayland) or xclip/xsel (X11)")
	default:
		return "", nil, fmt.Errorf("clipboard not supported on %s", runtime.GOOS)
	}
}

// pasteCommand returns the helper that prints the clipboard contents to stdout.
// Used internally only, to compare before an auto-clear — never exposed.
func pasteCommand() (string, []string, error) {
	switch runtime.GOOS {
	case "darwin":
		return "pbpaste", nil, nil
	case "linux":
		if os.Getenv("WAYLAND_DISPLAY") != "" {
			if p, err := exec.LookPath("wl-paste"); err == nil {
				return p, []string{"--no-newline"}, nil
			}
		}
		if p, err := exec.LookPath("xclip"); err == nil {
			return p, []string{"-selection", "clipboard", "-o"}, nil
		}
		if p, err := exec.LookPath("xsel"); err == nil {
			return p, []string{"--clipboard", "--output"}, nil
		}
		return "", nil, fmt.Errorf("no clipboard helper found")
	default:
		return "", nil, fmt.Errorf("clipboard not supported on %s", runtime.GOOS)
	}
}

// read returns the current clipboard contents. Private by design.
func read() (string, error) {
	name, args, err := pasteCommand()
	if err != nil {
		return "", err
	}
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
