package ipc

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"
)

type dirMeta struct {
	chatID     string
	role       string
	subAgentID string
	isAudit    bool // true if this is a session log file
}

type Watcher struct {
	router    *Router
	fsWatcher *fsnotify.Watcher
	metas     map[string]dirMeta
	mu        sync.RWMutex // protects metas
}

// NewWatcher creates a Watcher. Returns error if fsnotify cannot initialise.
func NewWatcher(r *Router) (*Watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("ipc: new watcher: %w", err)
	}
	return &Watcher{router: r, fsWatcher: fw, metas: make(map[string]dirMeta)}, nil
}

// RegisterAuditFile adds memoryRootDir/log.jsonl to the watch list.
func (w *Watcher) RegisterAuditFile(memRootDir, chatID string) error {
	logFile := filepath.Join(memRootDir, "log.jsonl")
	// Ensure file exists so we can watch it
	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_RDONLY, 0600)
	if err != nil {
		return fmt.Errorf("ipc: create audit log: %w", err)
	}
	f.Close()

	if err := w.fsWatcher.Add(logFile); err != nil {
		return fmt.Errorf("ipc: watch audit log %s: %w", logFile, err)
	}
	w.mu.Lock()
	w.metas[logFile] = dirMeta{chatID: chatID, isAudit: true}
	w.mu.Unlock()
	return nil
}

// RegisterDir adds ipcRootDir/messages/, /tasks/, /groups/, /agents/, and /reactions/ to the watch list.
// chatID is the authoritative chat ID for this IPC directory (derived from the filesystem path).
// Safe to call at any time, including after Watch has started.
func (w *Watcher) RegisterDir(ipcRootDir, chatID, role, subAgentID string) error {
	if chatID == "" {
		return fmt.Errorf("ipc: RegisterDir called with empty chatID for %s", ipcRootDir)
	}
	for _, sub := range []string{"messages", "tasks", "groups", "agents", "reactions"} {
		dir := filepath.Join(ipcRootDir, sub)
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("ipc: mkdir %s: %w", dir, err)
		}
		// MkdirAll only sets permissions on newly created dirs; chmod ensures
		// pre-existing dirs (e.g. from a previous run with 0755) are tightened too.
		if err := os.Chmod(dir, 0700); err != nil {
			return fmt.Errorf("ipc: chmod %s: %w", dir, err)
		}
		if err := w.fsWatcher.Add(dir); err != nil {
			return fmt.Errorf("ipc: watch %s: %w", dir, err)
		}
		w.mu.Lock()
		w.metas[dir] = dirMeta{chatID: chatID, role: role, subAgentID: subAgentID}
		w.mu.Unlock()
	}
	return nil
}

// Watch processes fsnotify events until ctx is cancelled.
func (w *Watcher) Watch(ctx context.Context) {
	defer w.fsWatcher.Close()
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-w.fsWatcher.Events:
			if !ok {
				return
			}
			w.mu.RLock()
			meta, ok := w.metas[event.Name]
			if !ok {
				// Try parent if it's a file inside a watched dir
				meta, ok = w.metas[filepath.Dir(event.Name)]
			}
			w.mu.RUnlock()

			if meta.isAudit {
				if event.Op&fsnotify.Write != 0 {
					// Audit log updated - in a real implementation we'd tail it
					log.Printf("audit: log.jsonl updated for %s", meta.chatID)
				}
				continue
			}

			if event.Op&fsnotify.Create == 0 {
				continue
			}

			parent := filepath.Dir(event.Name)
			subdir := filepath.Base(parent)
			if err := w.router.Route(subdir, event.Name, meta.chatID, meta.role, meta.subAgentID); err != nil {
				log.Printf("ipc: route %s: %v", event.Name, err)
				continue
			}
			os.Remove(event.Name)
		case err, ok := <-w.fsWatcher.Errors:
			if !ok {
				return
			}
			log.Printf("ipc: watcher error: %v", err)
		}
	}
}
