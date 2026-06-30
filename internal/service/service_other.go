//go:build !darwin && !linux

package service

import "fmt"

// On unsupported platforms there is no service manager; callers fall back to
// spawning the daemon directly.

func WriteDescriptor(binPath string) (bool, error) { return false, nil }

func Start(reload bool) error { return fmt.Errorf("managed service not supported on this OS") }

func Stop() error { return nil }

func Remove() error { return nil }

func Available() bool { return false }
