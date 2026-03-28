//go:build linux

package service

import "fmt"

// New returns the Manager implementation appropriate for the current platform.
// On Linux this uses systemd; full implementation is added in Task 3.
func New() (Manager, error) {
	return nil, fmt.Errorf("service: systemd lifecycle support not yet implemented")
}
