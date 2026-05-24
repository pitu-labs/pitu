package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/pitu-dev/pitu/internal/ipc"
)

// dispatchCapability writes a CapabilityRequest to ipc/requests/ and blocks until the
// harness writes ipc/responses/<requestID>.json (or timeout elapses). It polls the
// responses dir and consumes (deletes) the response file once read. Returns the
// Result, or an error carrying the response Error/ErrorCode.
func (h *toolHandlers) dispatchCapability(capability, tool string, params map[string]any, timeout time.Duration) (any, error) {
	requestID := uuid.NewString()
	respDir := filepath.Join(h.ipcDir, "responses")
	if err := os.MkdirAll(respDir, 0700); err != nil {
		return nil, fmt.Errorf("pitu-mcp: mkdir responses: %w", err)
	}
	respPath := filepath.Join(respDir, requestID+".json")

	req := ipc.CapabilityRequest{
		RequestID:  requestID,
		Capability: capability,
		Tool:       tool,
		Params:     params,
		ChatID:     h.chatID,
	}
	if err := h.writeIPC("requests", req); err != nil {
		return nil, err
	}

	deadline := time.After(timeout)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-deadline:
			return nil, fmt.Errorf("pitu-mcp: capability %s timeout after %s", tool, timeout)
		case <-ticker.C:
			data, err := os.ReadFile(respPath)
			if err != nil {
				continue // not written yet
			}
			os.Remove(respPath) // consume
			var resp ipc.CapabilityResponse
			if err := json.Unmarshal(data, &resp); err != nil {
				return nil, fmt.Errorf("pitu-mcp: unmarshal response: %w", err)
			}
			if resp.Error != "" {
				return nil, fmt.Errorf("%s (%s)", resp.Error, resp.ErrorCode)
			}
			return resp.Result, nil
		}
	}
}
