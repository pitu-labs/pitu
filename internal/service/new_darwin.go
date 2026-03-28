//go:build darwin

package service

import "fmt"

// New returns the Manager implementation appropriate for the current platform.
// On macOS this uses launchd; full implementation is added in Task 4/5.
func New() (Manager, error) {
	return nil, fmt.Errorf("service: launchd lifecycle support not yet implemented")
}
