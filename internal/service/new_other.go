//go:build !linux && !darwin

package service

import (
	"fmt"
	"runtime"
)

// New returns the Manager implementation appropriate for the current platform.
func New() (Manager, error) {
	return nil, fmt.Errorf("service: unsupported platform %q", runtime.GOOS)
}
