package service

import (
	"errors"
	"fmt"
	"runtime"
)

// ErrNotInstalled is returned when a lifecycle operation is attempted on an
// unregistered service.
var ErrNotInstalled = errors.New("pitu service is not installed — run 'pitu service install' first")

// Manager abstracts OS-native service management.
type Manager interface {
	Install() error
	Uninstall() error
	Start() error
	Stop() error
	Status() (string, error)
	Logs(n int) error
}

// New returns the Manager implementation appropriate for the current platform.
func New() (Manager, error) {
	switch runtime.GOOS {
	case "darwin":
		return newLaunchdManager()
	case "linux":
		return newSystemdManager()
	default:
		return nil, fmt.Errorf("service: unsupported platform %q", runtime.GOOS)
	}
}
