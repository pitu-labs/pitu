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

func TestPlistContent(t *testing.T) {
	got := service.PlistContent("/home/alice/pitu/pitu", "/home/alice")
	cases := []string{
		"dev.pitu.pitu",
		"<string>/home/alice/pitu/pitu</string>",
		"<key>RunAtLoad</key>",
		"<key>KeepAlive</key>",
		"/home/alice/.pitu/logs/pitu.log",
		"/home/alice/.pitu/logs/pitu.error.log",
		"<key>HOME</key>",
	}
	for _, want := range cases {
		if !strings.Contains(got, want) {
			t.Errorf("PlistContent() missing %q\ngot:\n%s", want, got)
		}
	}
}
