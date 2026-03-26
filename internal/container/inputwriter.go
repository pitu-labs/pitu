package container

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pitu-dev/pitu/internal/ipc"
)

// WriteInputFile writes msg as JSON to ipcRootDir/input/{ts}-{messageID}.json.
// Returns the full path of the written file.
func WriteInputFile(ipcRootDir string, msg ipc.InboundMessage) (string, error) {
	dir := filepath.Join(ipcRootDir, "input")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("container: mkdir input: %w", err)
	}
	ts := time.Now().UnixNano()
	name := fmt.Sprintf("%d-%s.json", ts, msg.MessageID)
	path := filepath.Join(dir, name)
	data, err := json.Marshal(msg)
	if err != nil {
		return "", fmt.Errorf("container: marshal message: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", fmt.Errorf("container: write input file: %w", err)
	}
	return path, nil
}
