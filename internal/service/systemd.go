package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// SystemdManager implements Manager for Linux systemd user sessions.
type SystemdManager struct{}

func newSystemdManager() (*SystemdManager, error) {
	if isWSL() {
		if err := exec.Command("systemctl", "--user", "daemon-reload").Run(); err != nil {
			return nil, fmt.Errorf(
				"systemd is not available in this WSL instance.\n" +
					"Enable it by adding to /etc/wsl.conf:\n\n" +
					"    [boot]\n    systemd=true\n\n" +
					"Then restart WSL from PowerShell: wsl --shutdown",
			)
		}
	}
	return &SystemdManager{}, nil
}

func isWSL() bool {
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return false
	}
	lower := strings.ToLower(string(data))
	return strings.Contains(lower, "microsoft") || strings.Contains(lower, "wsl")
}

func (m *SystemdManager) unitPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "systemd", "user", "pitu.service")
}

func (m *SystemdManager) isInstalled() bool {
	_, err := os.Stat(m.unitPath())
	return err == nil
}

// UnitContent returns the systemd unit file content for the given binary and home dir.
// Exported for testing; callers should use Install() for end-to-end behaviour.
func UnitContent(binary, home string) string {
	return fmt.Sprintf(`[Unit]
Description=Pitu Telegram agent harness
After=network.target

[Service]
Type=simple
ExecStart=%s
Restart=always
RestartSec=5
KillMode=process
Environment=HOME=%s
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=default.target
`, binary, home)
}

// Stub lifecycle methods — replaced with real implementations in Task 3.

func (m *SystemdManager) Install() error          { return fmt.Errorf("not implemented") }
func (m *SystemdManager) Uninstall() error        { return fmt.Errorf("not implemented") }
func (m *SystemdManager) Start() error            { return fmt.Errorf("not implemented") }
func (m *SystemdManager) Stop() error             { return fmt.Errorf("not implemented") }
func (m *SystemdManager) Status() (string, error) { return "", fmt.Errorf("not implemented") }
func (m *SystemdManager) Logs(n int) error        { return fmt.Errorf("not implemented") }
