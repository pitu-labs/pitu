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
			if event.Op&fsnotify.Create == 0 {
			        continue
			}
			parent := filepath.Dir(event.Name)
			w.mu.RLock()
			meta := w.metas[parent]
			w.mu.RUnlock()
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
