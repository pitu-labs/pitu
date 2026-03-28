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

func (m *SystemdManager) Install() error {
	binary, err := os.Executable()
	if err != nil {
		return fmt.Errorf("service: resolve binary path: %w", err)
	}
	if binary, err = filepath.EvalSymlinks(binary); err != nil {
		return fmt.Errorf("service: resolve binary symlink: %w", err)
	}
	home, _ := os.UserHomeDir()

	unitDir := filepath.Dir(m.unitPath())
	if err := os.MkdirAll(unitDir, 0700); err != nil {
		return fmt.Errorf("service: mkdir unit dir: %w", err)
	}
	if err := os.WriteFile(m.unitPath(), []byte(UnitContent(binary, home)), 0644); err != nil {
		return fmt.Errorf("service: write unit file: %w", err)
	}

	for _, args := range [][]string{
		{"--user", "daemon-reload"},
		{"--user", "enable", "pitu"},
		{"--user", "restart", "pitu"},
	} {
		if out, err := exec.Command("systemctl", args...).CombinedOutput(); err != nil {
			return fmt.Errorf("systemctl %s: %w\n%s", strings.Join(args, " "), err, out)
		}
	}

	// Enable linger so the user service survives logout and starts on boot.
	if out, err := exec.Command("loginctl", "enable-linger").CombinedOutput(); err != nil {
		fmt.Printf("warning: loginctl enable-linger failed (service may stop on logout): %s\n", out)
	}

	fmt.Printf("pitu service installed and started.\nUnit: %s\n", m.unitPath())
	return nil
}

func (m *SystemdManager) Uninstall() error {
	if !m.isInstalled() {
		fmt.Println("pitu service is not installed — nothing to do.")
		return nil
	}
	for _, args := range [][]string{
		{"--user", "stop", "pitu"},
		{"--user", "disable", "pitu"},
	} {
		exec.Command("systemctl", args...).Run() // best-effort
	}
	if err := os.Remove(m.unitPath()); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("service: remove unit file: %w", err)
	}
	exec.Command("systemctl", "--user", "daemon-reload").Run() // best-effort
	fmt.Println("pitu service uninstalled.")
	return nil
}

func (m *SystemdManager) Start() error {
	if !m.isInstalled() {
		return ErrNotInstalled
	}
	if out, err := exec.Command("systemctl", "--user", "start", "pitu").CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl start: %w\n%s", err, out)
	}
	fmt.Println("pitu service started.")
	return nil
}

func (m *SystemdManager) Stop() error {
	if !m.isInstalled() {
		return ErrNotInstalled
	}
	if out, err := exec.Command("systemctl", "--user", "stop", "pitu").CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl stop: %w\n%s", err, out)
	}
	fmt.Println("pitu service stopped.")
	return nil
}

func (m *SystemdManager) Status() (string, error) {
	if !m.isInstalled() {
		return "not installed", nil
	}
	out, _ := exec.Command("systemctl", "--user", "is-active", "pitu").Output()
	return strings.TrimSpace(string(out)), nil
}

func (m *SystemdManager) Logs(n int) error {
	if !m.isInstalled() {
		return ErrNotInstalled
	}
	cmd := exec.Command("journalctl", "--user", "-u", "pitu", "-f", fmt.Sprintf("-n%d", n))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
