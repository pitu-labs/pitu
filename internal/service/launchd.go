package service

import (
	"fmt"
	"os"
	"path/filepath"
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

// Stub lifecycle methods — replaced with real implementations in Task 5.

func (m *LaunchdManager) Install() error          { return fmt.Errorf("not implemented") }
func (m *LaunchdManager) Uninstall() error        { return fmt.Errorf("not implemented") }
func (m *LaunchdManager) Start() error            { return fmt.Errorf("not implemented") }
func (m *LaunchdManager) Stop() error             { return fmt.Errorf("not implemented") }
func (m *LaunchdManager) Status() (string, error) { return "", fmt.Errorf("not implemented") }
func (m *LaunchdManager) Logs(n int) error        { return fmt.Errorf("not implemented") }
