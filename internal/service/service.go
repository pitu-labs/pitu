package service

import (
	"errors"
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
	Status() (string, error) // returns human-readable state: "active", "inactive", "not installed"
	Logs(n int) error        // streams last n lines then follows; blocks until interrupted
}
