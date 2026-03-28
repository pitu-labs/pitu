package container

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

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

type Manager struct {
	cfg        *config.Config
	pool       sync.Map // chatID → *Handle
	skillsDisc []skills.Skill
	watcher    interface{ RegisterDir(string) error } // ipc.Watcher, accepts nil
	onExpire   func(chatID string)

	// Dirs used by all containers
	skillsDir string // host path for /workspace/skills
	dataDir   string // host base path; per-chat subdirs created here
}

func NewManager(cfg *config.Config, discovered []skills.Skill, w interface{ RegisterDir(string) error }, onExpire func(string)) *Manager {
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

	args := m.BuildRunArgs(chatID, ipcDir, memDir, m.skillsDir, opencodeDir)
	out, err := exec.CommandContext(ctx, "podman", args...).Output()
	if err != nil {
		return nil, fmt.Errorf("podman run: %w (output: %s)", err, out)
	}
	containerID := string(out)
	containerID = containerID[:len(containerID)-1] // trim newline

	// Register the new container's IPC dirs with the watcher
	if m.watcher != nil {
		if err := m.watcher.RegisterDir(ipcDir); err != nil {
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

func (m *Manager) execOpenCode(ctx context.Context, handle *Handle, inputPath string) error {
	args := m.BuildExecArgs(handle.ID, inputPath, handle.hasSession)
	out, err := exec.CommandContext(ctx, "podman", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("podman exec opencode: %w (output: %s)", err, out)
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

// StopAll stops all warm containers. Call on harness shutdown.
func (m *Manager) StopAll() {
	m.pool.Range(func(k, v any) bool {
		h := v.(*Handle)
		h.ttlTimer.Stop()
		exec.Command("podman", "stop", h.ID).Run()
		m.pool.Delete(k)
		return true
	})
}

// BuildRunArgs returns the podman run arguments for a new container. Public for testability.
func (m *Manager) BuildRunArgs(chatID, ipcDir, memDir, skillsDir, opencodeDir string) []string {
	opencodeCfg := GenerateOpenCodeConfig(chatID)
	return []string{
		"run", "--detach", "--rm",
		"--memory", m.cfg.Container.MemoryLimit,
		"--env", "PITU_CHAT_ID=" + chatID,
		"--env", "OPENCODE_CONFIG_CONTENT=" + opencodeCfg,
		"--volume", ipcDir + ":/workspace/ipc:z",
		"--volume", memDir + ":/workspace/memory:z",
		"--volume", skillsDir + ":/workspace/skills:ro,z",
		"--volume", opencodeDir + ":/root/.local/share/opencode:z",
		m.cfg.Container.Image,
		"sleep", "infinity", // container stays alive; OpenCode invoked per-message via exec
	}
}

// BuildExecArgs returns the podman exec arguments for running OpenCode on a message. Public for testability.
func (m *Manager) BuildExecArgs(containerID, inputPath string, continueSession bool) []string {
	containerPath := "/workspace/ipc/input/" + filepath.Base(inputPath)
	args := []string{"exec", containerID, "opencode", "run"}
	if continueSession {
		args = append(args, "-c")
	}
	args = append(args, "-f", containerPath, "--", "Process the attached inbound message and respond via mcp__pitu__sendMessage.")
	return args
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

// SpawnSubAgent runs a one-shot OpenCode sub-agent inside the container for chatID.
// It runs in a goroutine so the caller is not blocked. The sub-agent inherits ctx,
// so it is cancelled if the application shuts down.
func (m *Manager) SpawnSubAgent(ctx context.Context, chatID, role, prompt string) {
	v, ok := m.pool.Load(chatID)
	if !ok {
		log.Printf("container: SpawnSubAgent: no container for chat %s", chatID)
		return
	}
	handle := v.(*Handle)
	args := m.BuildSpawnArgs(handle.ID, role, prompt)
	go func() {
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
