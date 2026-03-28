//go:build !darwin

package service

import "fmt"

// LaunchdManager is a stub on non-darwin platforms.
// The real implementation is in launchd.go (darwin only).
// Exported only to satisfy the compile-time interface check in service_test.go.
type LaunchdManager struct{}

func newLaunchdManager() (*LaunchdManager, error) {
	return nil, fmt.Errorf("service: launchd is only available on macOS")
}

// Install, Uninstall, Start, Stop, Status, and Logs are stubs on non-darwin.
// Full implementations are added in Task 4/5.

func (m *LaunchdManager) Install() error          { return fmt.Errorf("not implemented") }
func (m *LaunchdManager) Uninstall() error        { return fmt.Errorf("not implemented") }
func (m *LaunchdManager) Start() error            { return fmt.Errorf("not implemented") }
func (m *LaunchdManager) Stop() error             { return fmt.Errorf("not implemented") }
func (m *LaunchdManager) Status() (string, error) { return "", fmt.Errorf("not implemented") }
func (m *LaunchdManager) Logs(_ int) error        { return fmt.Errorf("not implemented") }
