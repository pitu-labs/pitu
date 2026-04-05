package container

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pitu-dev/pitu/internal/config"
	"github.com/pitu-dev/pitu/internal/ipc"
	"github.com/pitu-dev/pitu/internal/skills"
)

type Handle struct {
	ID           string
	IPCDir       string
	hasSession   bool
	lastActivity time.Time
	ttlTimer     *time.Timer
}

type SubAgentHandle struct {
	ID         string
	ChatID     string
	Role       string
	SubAgentID string
	IPCDir     string
	ttlTimer   *time.Timer
}

type Manager struct {
	cfg        *config.Config
	pool       sync.Map // chatID → *Handle
	subPool    sync.Map // subAgentID -> *SubAgentHandle
	skillsDisc []skills.Skill
	watcher    interface{ RegisterDir(string, string, string, string) error } // ipc.Watcher, accepts nil
	onExpire   func(chatID string)

	startMu sync.Mutex // serialises startContainer; prevents duplicate starts on concurrent messages

	// Dirs used by all containers
	skillsDir string // host path for /workspace/skills
	dataDir   string // host base path; per-chat subdirs created here
}

func NewManager(cfg *config.Config, discovered []skills.Skill, w interface{ RegisterDir(string, string, string, string) error }, onExpire func(string)) *Manager {
	return &Manager{cfg: cfg, skillsDisc: discovered, watcher: w, onExpire: onExpire}
}

// SetDirs configures the host-side directory roots. Call before Dispatch.
func (m *Manager) SetDirs(dataDir, skillsDir string) {
	m.dataDir = dataDir
	m.skillsDir = skillsDir
}

// Dispatch handles an inbound message: reuses a warm container or starts a new one,
// writes the input file, then runs OpenCode inside the container.
func (m *Manager) Dispatch(ctx context.Context, chatID string, msg ipc.InboundMessage) error {
	handle, err := m.ensureContainer(ctx, chatID)
	if err != nil {
		return fmt.Errorf("container: ensure: %w", err)
	}
	handle.lastActivity = time.Now()
	handle.ttlTimer.Reset(m.ttl())

	inputPath, err := WriteInputFile(handle.IPCDir, msg)
	if err != nil {
		return err
	}
	return m.execOpenCode(ctx, handle, inputPath)
}

func (m *Manager) ensureContainer(ctx context.Context, chatID string) (*Handle, error) {
	// Fast path: container already warm — no lock needed.
	if v, ok := m.pool.Load(chatID); ok {
		return v.(*Handle), nil
	}
	// Slow path: serialise to prevent duplicate starts for the same chatID.
	m.startMu.Lock()
	defer m.startMu.Unlock()
	// Re-check: another goroutine may have started it while we waited for the lock.
	if v, ok := m.pool.Load(chatID); ok {
		return v.(*Handle), nil
	}
	return m.startContainer(ctx, chatID)
}

func (m *Manager) startContainer(ctx context.Context, chatID string) (*Handle, error) {
	ipcDir := filepath.Join(m.dataDir, chatID, "ipc")
	memDir := filepath.Join(m.dataDir, chatID, "memory")
	opencodeDir := filepath.Join(m.dataDir, chatID, "opencode")
	if err := os.MkdirAll(ipcDir, 0700); err != nil {
		return nil, fmt.Errorf("mkdir ipc: %w", err)
	}
	if err := os.MkdirAll(memDir, 0700); err != nil {
		return nil, fmt.Errorf("mkdir memory: %w", err)
	}
	if err := os.MkdirAll(opencodeDir, 0700); err != nil {
		return nil, fmt.Errorf("mkdir opencode: %w", err)
	}

	ef, err := os.CreateTemp("", "pitu-opencode-env-*")
	if err != nil {
		return nil, fmt.Errorf("container: create env file: %w", err)
	}
	envFilePath := ef.Name()
	// podman run --detach reads --env-file synchronously before the client returns,
	// so the file is safe to remove once startContainer returns.
	defer os.Remove(envFilePath)
	if err := ef.Chmod(0600); err != nil {
		ef.Close()
		return nil, fmt.Errorf("container: chmod env file: %w", err)
	}
	if err := m.writeEnvFile(ef, chatID); err != nil {
		ef.Close()
		return nil, err
	}
	if err := ef.Close(); err != nil {
		return nil, fmt.Errorf("container: close env file: %w", err)
	}

	args := m.BuildRunArgs(chatID, ipcDir, memDir, m.skillsDir, opencodeDir, envFilePath)
	cmd := exec.CommandContext(ctx, "podman", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("podman run: %w (stderr: %s)", err, stderr.String())
	}
	containerID := string(out)
	containerID = containerID[:len(containerID)-1] // trim newline

	// Register the new container's IPC dirs with the watcher
	if m.watcher != nil {
		if err := m.watcher.RegisterDir(ipcDir, chatID, "", ""); err != nil {
			log.Printf("container: register ipc dirs for %s: %v", chatID, err)
		}
	}

	handle := &Handle{ID: containerID, IPCDir: ipcDir, lastActivity: time.Now()}
	handle.ttlTimer = time.AfterFunc(m.ttl(), func() {
		m.stopContainer(chatID, containerID)
	})
	m.pool.Store(chatID, handle)
	return handle, nil
}

func (m *Manager) startSubAgentContainer(ctx context.Context, chatID, role, subAgentID string) (*SubAgentHandle, error) {
	agentRoot := filepath.Join(m.dataDir, chatID, "agents", subAgentID)
	ipcDir := filepath.Join(agentRoot, "ipc")
	memDir := filepath.Join(agentRoot, "memory")
	skillsDir := filepath.Join(agentRoot, "skills")
	opencodeDir := filepath.Join(agentRoot, "opencode")

	dirs := []string{ipcDir, memDir, skillsDir, opencodeDir, filepath.Join(skillsDir, "system")}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0700); err != nil {
			return nil, fmt.Errorf("mkdir %s: %w", d, err)
		}
	}

	// Write system.md skill
	systemSkillPath := filepath.Join(skillsDir, "system", "SKILL.md")
	systemSkillContent := fmt.Sprintf("# System\n\nYou are a sub-agent with the role: %s. Your ChatID is %s and your SubAgentID is %s.\n", role, chatID, subAgentID)
	if err := os.WriteFile(systemSkillPath, []byte(systemSkillContent), 0600); err != nil {
		return nil, fmt.Errorf("write system skill: %w", err)
	}

	ef, err := os.CreateTemp("", "pitu-subagent-env-*")
	if err != nil {
		return nil, fmt.Errorf("container: create env file: %w", err)
	}
	envFilePath := ef.Name()
	defer os.Remove(envFilePath)
	if err := ef.Chmod(0600); err != nil {
		ef.Close()
		return nil, fmt.Errorf("container: chmod env file: %w", err)
	}
	if err := m.writeEnvFile(ef, chatID); err != nil {
		ef.Close()
		return nil, err
	}
	if err := ef.Close(); err != nil {
		return nil, fmt.Errorf("container: close env file: %w", err)
	}

	args := m.BuildSubAgentRunArgs(chatID, subAgentID, role, ipcDir, memDir, skillsDir, opencodeDir, envFilePath)
	cmd2 := exec.CommandContext(ctx, "podman", args...)
	var stderr2 bytes.Buffer
	cmd2.Stderr = &stderr2
	out, err := cmd2.Output()
	if err != nil {
		return nil, fmt.Errorf("podman run subagent: %w (stderr: %s)", err, stderr2.String())
	}
	containerID := string(out)
	containerID = containerID[:len(containerID)-1] // trim newline

	if m.watcher != nil {
		if err := m.watcher.RegisterDir(ipcDir, chatID, role, subAgentID); err != nil {
			log.Printf("container: register ipc dirs for subagent %s: %v", subAgentID, err)
		}
	}

	handle := &SubAgentHandle{
		ID:         containerID,
		ChatID:     chatID,
		Role:       role,
		SubAgentID: subAgentID,
		IPCDir:     ipcDir,
	}
	handle.ttlTimer = time.AfterFunc(m.ttl(), func() {
		m.stopSubAgentContainer(subAgentID, containerID)
	})
	m.subPool.Store(subAgentID, handle)
	return handle, nil
}

func (m *Manager) execOpenCode(ctx context.Context, handle *Handle, inputPath string) error {
	args := m.BuildExecArgs(handle.ID, inputPath, handle.hasSession)
	out, err := exec.CommandContext(ctx, "podman", args...).CombinedOutput()
	if len(out) > 0 {
		log.Printf("opencode output (container %s): %s", handle.ID[:12], out)
	}
	if err != nil {
		return fmt.Errorf("podman exec opencode: %w", err)
	}
	handle.hasSession = true
	return nil
}

func (m *Manager) stopContainer(chatID, containerID string) {
	exec.Command("podman", "stop", containerID).Run()
	m.pool.Delete(chatID)
	if m.onExpire != nil {
		m.onExpire(chatID)
	}
	log.Printf("container: stopped %s (chat %s TTL expired)", containerID, chatID)
}

func (m *Manager) stopSubAgentContainer(subAgentID, containerID string) {
	exec.Command("podman", "stop", containerID).Run()
	m.subPool.Delete(subAgentID)
	log.Printf("container: stopped %s (sub-agent %s TTL expired)", containerID, subAgentID)
}

// StopAll stops all warm containers. Call on harness shutdown.
func (m *Manager) StopAll() {
	m.pool.Range(func(k, v any) bool {
		h := v.(*Handle)
		h.ttlTimer.Stop()
		exec.Command("podman", "stop", h.ID).Run()
		m.pool.Delete(k)
		return true
	})
	m.subPool.Range(func(k, v any) bool {
		h := v.(*SubAgentHandle)
		h.ttlTimer.Stop()
		exec.Command("podman", "stop", h.ID).Run()
		m.subPool.Delete(k)
		return true
	})
}

// BuildRunArgs returns the podman run arguments for a new container. Public for testability.
func (m *Manager) BuildRunArgs(chatID, ipcDir, memDir, skillsDir, opencodeDir, envFile string) []string {
	return []string{
		"run", "--detach", "--rm",
		"--memory", m.cfg.Container.MemoryLimit,
		"--env", "PITU_CHAT_ID=" + chatID,
		"--env-file", envFile,
		"--volume", ipcDir + ":/workspace/ipc:z",
		"--volume", memDir + ":/workspace/memory:z",
		"--volume", skillsDir + ":/workspace/skills:ro,z",
		"--volume", opencodeDir + ":/root/.local/share/opencode:z",
		m.cfg.Container.Image,
		"sleep", "infinity", // container stays alive; OpenCode invoked per-message via exec
	}
}

// BuildSubAgentRunArgs returns the podman run arguments for a new sub-agent container. Public for testability.
func (m *Manager) BuildSubAgentRunArgs(chatID, subAgentID, role, ipcDir, memDir, skillsDir, opencodeDir, envFile string) []string {
	return []string{
		"run", "--detach", "--rm",
		"--memory", m.cfg.Container.MemoryLimit,
		"--env", "PITU_CHAT_ID=" + chatID,
		"--env", "PITU_SUB_AGENT_ID=" + subAgentID,
		"--env", "PITU_ROLE=" + role,
		"--env-file", envFile,
		"--volume", ipcDir + ":/workspace/ipc:z",
		"--volume", memDir + ":/workspace/memory:z",
		"--volume", skillsDir + ":/workspace/skills:ro,z",
		"--volume", opencodeDir + ":/root/.local/share/opencode:z",
		m.cfg.Container.Image,
		"sleep", "infinity",
	}
}

// BuildExecArgs returns the podman exec arguments for running OpenCode on a message. Public for testability.
func (m *Manager) BuildExecArgs(containerID, inputPath string, continueSession bool) []string {
	if m.cfg.Container.Runtime == "pimono" {
		return m.BuildExecArgsPimono(containerID, inputPath)
	}
	containerPath := "/workspace/ipc/input/" + filepath.Base(inputPath)
	// --workdir places OpenCode in the memory dir so it discovers AGENTS.md via
	// its standard upward traversal — no vendor-specific config needed.
	args := []string{"exec", "--workdir", "/workspace/memory", containerID, "opencode", "run"}
	if continueSession {
		args = append(args, "-c")
	}
	args = append(args, "-f", containerPath, "--", "Process the inbound message from the input file.")
	return args
}

// BuildExecArgsPimono returns the podman exec arguments for running Pi-Mono on a message. Public for testability.
func (m *Manager) BuildExecArgsPimono(containerID, inputPath string) []string {
	containerPath := "/workspace/ipc/input/" + filepath.Base(inputPath)
	return []string{"exec", containerID, "pi", "run", "--file", containerPath}
}

// BuildSpawnArgs returns the podman exec arguments for running a sub-agent. Public for testability.
func (m *Manager) BuildSpawnArgs(containerID, role, prompt string) []string {
	return []string{
		"exec", containerID,
		"opencode", "run",
		"--title", role,
		"--", role + ": " + prompt,
	}
}

// SpawnSubAgent runs a one-shot OpenCode sub-agent inside a fresh isolated container.
func (m *Manager) SpawnSubAgent(ctx context.Context, chatID, role, prompt string) {
	role = SanitizeRole(role)
	prompt = SanitizePrompt(prompt)
	subAgentID := uuid.NewString()
	go func() {
		handle, err := m.startSubAgentContainer(ctx, chatID, role, subAgentID)
		if err != nil {
			log.Printf("container: SpawnSubAgent: start: %v", err)
			return
		}

		args := m.BuildSpawnArgs(handle.ID, role, prompt)
		out, err := exec.CommandContext(ctx, "podman", args...).CombinedOutput()
		if err != nil {
			log.Printf("container: sub-agent %s (chat %s): %v (output: %s)", role, chatID, err, out)
		}
	}()
}
func (m *Manager) ttl() time.Duration {
	d, err := time.ParseDuration(m.cfg.Container.TTL)
	if err != nil {
		return 5 * time.Minute
	}
	return d
}

// SanitizeRole strips characters outside [a-zA-Z0-9 _-], caps the result at
// 64 runes, and returns "agent" if nothing survives. Exported for testability.
func SanitizeRole(role string) string {
	var b strings.Builder
	for _, r := range role {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == ' ' || r == '_' || r == '-' {
			b.WriteRune(r)
		}
	}
	runes := []rune(b.String())
	if len(runes) > 64 {
		runes = runes[:64]
	}
	if len(runes) == 0 {
		return "agent"
	}
	return string(runes)
}

// SanitizePrompt caps prompt at 4096 runes. Exported for testability.
func SanitizePrompt(prompt string) string {
	runes := []rune(prompt)
	if len(runes) > 4096 {
		return string(runes[:4096])
	}
	return prompt
}

// writeEnvFile writes OPENCODE_CONFIG_CONTENT and, when an API key is configured,
// the provider-specific key env var (e.g. ANTHROPIC_API_KEY) to ef.
// Keeping the key out of the config JSON reduces the blast radius of config leaks.
func (m *Manager) writeEnvFile(ef *os.File, chatID string) error {
	var configKey string
	var configContent string

	if m.cfg.Container.Runtime == "pimono" {
		configKey = "PI_CONFIG_CONTENT"
		configContent = GeneratePiMonoConfig(chatID, m.cfg.Model)
	} else {
		configKey = "OPENCODE_CONFIG_CONTENT"
		configContent = GenerateOpenCodeConfig(chatID, m.cfg.Model)
	}

	if _, err := fmt.Fprintf(ef, "%s=%s\n", configKey, configContent); err != nil {
		return fmt.Errorf("container: write config: %w", err)
	}
	if m.cfg.Model.APIKey != "" {
		keyVar := APIKeyEnvVar(m.cfg.Model.Provider)
		if _, err := fmt.Fprintf(ef, "%s=%s\n", keyVar, m.cfg.Model.APIKey); err != nil {
			return fmt.Errorf("container: write api key env var: %w", err)
		}
	}
	return nil
}
