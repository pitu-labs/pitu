package service

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
)

// LaunchdManager implements Manager for macOS launchd.
type LaunchdManager struct{}

func newLaunchdManager() (*LaunchdManager, error) {
	return &LaunchdManager{}, nil
}

func (m *LaunchdManager) plistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", "dev.pitu.pitu.plist")
}

func (m *LaunchdManager) logPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".pitu", "logs", "pitu.log")
}

func (m *LaunchdManager) errLogPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".pitu", "logs", "pitu.error.log")
}

func (m *LaunchdManager) isInstalled() bool {
	_, err := os.Stat(m.plistPath())
	return err == nil
}

// PlistContent returns the launchd plist XML for the given binary and home dir.
// Exported for testing; callers should use Install() for end-to-end behaviour.
// Full implementation added in Task 4.
func PlistContent(binary, home string) string {
	logPath := filepath.Join(home, ".pitu", "logs", "pitu.log")
	errLogPath := filepath.Join(home, ".pitu", "logs", "pitu.error.log")
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>dev.pitu.pitu</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>EnvironmentVariables</key>
  <dict>
    <key>HOME</key>
    <string>%s</string>
    <key>PATH</key>
    <string>/usr/local/bin:/usr/bin:/bin:%s/.local/bin</string>
  </dict>
  <key>StandardOutPath</key>
  <string>%s</string>
  <key>StandardErrorPath</key>
  <string>%s</string>
</dict>
</plist>
`, binary, home, home, logPath, errLogPath)
}

func (m *LaunchdManager) serviceTarget() string {
	return fmt.Sprintf("gui/%d", os.Getuid())
}

func (m *LaunchdManager) Install() error {
	binary, err := os.Executable()
	if err != nil {
		return fmt.Errorf("service: resolve binary path: %w", err)
	}
	if binary, err = filepath.EvalSymlinks(binary); err != nil {
		return fmt.Errorf("service: resolve binary symlink: %w", err)
	}
	home, _ := os.UserHomeDir()

	// Ensure log directory exists before launchd tries to open the log files.
	if err := os.MkdirAll(filepath.Join(home, ".pitu", "logs"), 0700); err != nil {
		return fmt.Errorf("service: mkdir log dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(m.plistPath()), 0700); err != nil {
		return fmt.Errorf("service: mkdir LaunchAgents: %w", err)
	}

	// Remove any existing registration before overwriting the plist.
	exec.Command("launchctl", "bootout", m.serviceTarget(), m.plistPath()).Run() // best-effort

	if err := os.WriteFile(m.plistPath(), []byte(PlistContent(binary, home)), 0644); err != nil {
		return fmt.Errorf("service: write plist: %w", err)
	}
	if out, err := exec.Command("launchctl", "bootstrap", m.serviceTarget(), m.plistPath()).CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl bootstrap: %w\n%s", err, out)
	}

	m.writeNewsyslogConfig()

	fmt.Printf("pitu service installed and started.\nPlist: %s\nLogs:  %s\n", m.plistPath(), m.logPath())
	return nil
}

// writeNewsyslogConfig installs a newsyslog rotation config if the Homebrew
// newsyslog.d directory is present. Silently skips if absent.
func (m *LaunchdManager) writeNewsyslogConfig() {
	newsyslogDir := "/usr/local/etc/newsyslog.d"
	if _, err := os.Stat(newsyslogDir); err != nil {
		return
	}
	u, err := user.Current()
	if err != nil {
		return
	}
	g, err := user.LookupGroupId(u.Gid)
	if err != nil {
		return
	}
	config := fmt.Sprintf("# pitu log rotation (10 MB cap, 5 backups)\n"+
		"%s\t%s:%s\t644\t5\t10240\t*\n"+
		"%s\t%s:%s\t644\t5\t10240\t*\n",
		m.logPath(), u.Username, g.Name,
		m.errLogPath(), u.Username, g.Name,
	)
	dest := filepath.Join(newsyslogDir, "pitu.conf")
	os.WriteFile(dest, []byte(config), 0644) // best-effort
}

func (m *LaunchdManager) Uninstall() error {
	if !m.isInstalled() {
		fmt.Println("pitu service is not installed — nothing to do.")
		return nil
	}
	exec.Command("launchctl", "bootout", m.serviceTarget(), m.plistPath()).Run() // best-effort
	if err := os.Remove(m.plistPath()); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("service: remove plist: %w", err)
	}
	// Remove newsyslog config if present.
	os.Remove("/usr/local/etc/newsyslog.d/pitu.conf")
	fmt.Println("pitu service uninstalled.")
	return nil
}

func (m *LaunchdManager) Start() error {
	if !m.isInstalled() {
		return ErrNotInstalled
	}
	// Use bootstrap (modern API) to re-register and start the service.
	if out, err := exec.Command("launchctl", "bootstrap", m.serviceTarget(), m.plistPath()).CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl bootstrap: %w\n%s", err, out)
	}
	fmt.Println("pitu service started.")
	return nil
}

func (m *LaunchdManager) Stop() error {
	if !m.isInstalled() {
		return ErrNotInstalled
	}
	// Use bootout (modern API) to unload and stop the service.
	// With KeepAlive=true, launchctl stop immediately restarts the process;
	// bootout removes it from launchd entirely, honouring the stop request.
	if out, err := exec.Command("launchctl", "bootout", m.serviceTarget(), m.plistPath()).CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl bootout: %w\n%s", err, out)
	}
	fmt.Println("pitu service stopped.")
	return nil
}

func (m *LaunchdManager) Status() (string, error) {
	if !m.isInstalled() {
		return "not installed", nil
	}
	out, err := exec.Command("launchctl", "list", "dev.pitu.pitu").Output()
	if err != nil {
		return "inactive (not loaded)", nil
	}
	// Output has a header row then: PID \t ExitStatus \t Label
	// A numeric PID (not "-") means the process is running.
	for line := range strings.SplitSeq(string(out), "\n") {
		if strings.Contains(line, "dev.pitu.pitu") {
			fields := strings.Fields(line)
			if len(fields) > 0 && fields[0] != "-" {
				return "active (PID " + fields[0] + ")", nil
			}
			return "loaded (not running)", nil
		}
	}
	return "loaded", nil
}

func (m *LaunchdManager) Logs(n int) error {
	if !m.isInstalled() {
		return ErrNotInstalled
	}
	cmd := exec.Command("tail", "-f", fmt.Sprintf("-n%d", n), m.logPath())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
