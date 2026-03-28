package service_test

import (
	"strings"
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

func TestUnitContent(t *testing.T) {
	got := service.UnitContent("/home/alice/pitu/pitu", "/home/alice")
	cases := []string{
		"ExecStart=/home/alice/pitu/pitu",
		"Environment=HOME=/home/alice",
		"StandardOutput=journal",
		"StandardError=journal",
		"Restart=always",
		"RestartSec=5",
		"WantedBy=default.target",
		"After=network.target",
	}
	for _, want := range cases {
		if !strings.Contains(got, want) {
			t.Errorf("UnitContent() missing %q\ngot:\n%s", want, got)
		}
	}
}
