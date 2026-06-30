package clipboard

import (
	"runtime"
	"testing"
	"time"
)

// TestCopyWithClear_OnlyClearsIfUnchanged proves the auto-clear never wipes
// content the user copied after Yucca: it clears only when the clipboard
// still holds the value Yucca put there. Saves and restores the real
// clipboard, and skips where no clipboard helper is available (e.g. CI).
func TestCopyWithClear_OnlyClearsIfUnchanged(t *testing.T) {
	if err := Copy("yucca-clip-probe"); err != nil {
		t.Skip("clipboard helper unavailable:", err)
	}
	saved, _ := Read()
	defer func() { _ = Copy(saved) }()

	const window = 150 * time.Millisecond

	// Unchanged → cleared.
	if err := CopyWithClear("secret-value", window); err != nil {
		t.Fatal(err)
	}
	time.Sleep(window + 250*time.Millisecond)
	if cur, _ := Read(); cur != "" {
		t.Errorf("clipboard should be cleared when unchanged, got %q", cur)
	}

	// User copied something else before the window → preserved.
	if err := CopyWithClear("secret-value", window); err != nil {
		t.Fatal(err)
	}
	_ = Copy("user-copied-this")
	time.Sleep(window + 250*time.Millisecond)
	if cur, _ := Read(); cur != "user-copied-this" {
		t.Errorf("user content must be preserved, got %q", cur)
	}
}

// TestCommandSelection verifies a backend is chosen for the current platform
// without touching the real clipboard.
func TestCommandSelection(t *testing.T) {
	copyName, _, copyErr := copyCommand()
	pasteName, _, pasteErr := pasteCommand()

	switch runtime.GOOS {
	case "darwin":
		if copyErr != nil || copyName != "pbcopy" {
			t.Errorf("darwin copy: got (%q, %v), want pbcopy", copyName, copyErr)
		}
		if pasteErr != nil || pasteName != "pbpaste" {
			t.Errorf("darwin paste: got (%q, %v), want pbpaste", pasteName, pasteErr)
		}
	case "linux":
		// A helper may or may not be installed in CI; either a resolved path
		// or a descriptive error is acceptable — just never a silent empty name.
		if copyErr == nil && copyName == "" {
			t.Error("linux copy: empty command with no error")
		}
	default:
		if copyErr == nil {
			t.Errorf("unsupported OS %s should return an error", runtime.GOOS)
		}
	}
}
