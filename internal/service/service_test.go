package service_test

import (
	"testing"

	"github.com/pitu-dev/pitu/internal/service"
)

// TestManagerInterface verifies that both platform implementations satisfy Manager.
// This is a compile-time check — if the interface is not satisfied, the build fails.
func TestManagerInterface(t *testing.T) {
	t.Skip("compile-time interface check only")
	var _ service.Manager = (*service.SystemdManager)(nil)
	var _ service.Manager = (*service.LaunchdManager)(nil)
}
