//go:build darwin || linux

package service

import "os"

// writeIfChanged writes content to path only if it differs from what's there,
// so re-running install never needlessly reloads a healthy service.
func writeIfChanged(path, content string, perm os.FileMode) (bool, error) {
	if existing, err := os.ReadFile(path); err == nil && string(existing) == content {
		return false, nil
	}
	if err := os.WriteFile(path, []byte(content), perm); err != nil {
		return false, err
	}
	return true, nil
}
