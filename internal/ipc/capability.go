package ipc

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// CapabilityRequest is written by pitu-mcp to ipc/requests/. It asks the harness
// to perform a capability-backed operation (e.g. a Gmail API call) and return the
// result. The harness reads it, dispatches by Capability, and writes a
// CapabilityResponse to ipc/responses/<RequestID>.json.
type CapabilityRequest struct {
	RequestID  string         `json:"request_id"`
	Capability string         `json:"capability"`
	Tool       string         `json:"tool"`
	Params     map[string]any `json:"params"`
	ChatID     string         `json:"chat_id"` // overwritten by router from path
}

// CapabilityResponse is written by the harness to ipc/responses/<RequestID>.json.
// pitu-mcp blocks until the file appears, reads it, and returns the result to the agent.
type CapabilityResponse struct {
	RequestID string `json:"request_id"`
	Result    any    `json:"result,omitempty"`
	Error     string `json:"error,omitempty"`
	ErrorCode string `json:"error_code,omitempty"`
}

// WriteResponse writes resp to ipcRoot/responses/<RequestID>.json using the
// atomic temp-then-rename pattern (a temp file in ipcRoot, renamed into the
// responses subdir). responses/ is NOT watched by the harness — pitu-mcp inside
// the container watches it. The harness creates the dir if missing.
func WriteResponse(ipcRoot string, resp CapabilityResponse) error {
	respDir := filepath.Join(ipcRoot, "responses")
	if err := os.MkdirAll(respDir, 0700); err != nil {
		return fmt.Errorf("ipc: mkdir responses: %w", err)
	}
	data, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("ipc: marshal response: %w", err)
	}
	tmp, err := os.CreateTemp(ipcRoot, ".tmp-")
	if err != nil {
		return fmt.Errorf("ipc: create temp: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("ipc: write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("ipc: close temp: %w", err)
	}
	dest := filepath.Join(respDir, resp.RequestID+".json")
	if err := os.Rename(tmpPath, dest); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("ipc: rename response: %w", err)
	}
	return nil
}
