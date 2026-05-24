package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pitu-dev/pitu/internal/ipc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDispatchCapability_RoundTrip(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "requests"), 0700))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "responses"), 0700))

	h := &toolHandlers{ipcDir: root, chatID: "chat-1"}

	// Simulate the harness: watch requests/, write a response keyed by the request_id.
	go func() {
		var reqID string
		for i := 0; i < 200 && reqID == ""; i++ {
			entries, _ := os.ReadDir(filepath.Join(root, "requests"))
			for _, e := range entries {
				data, _ := os.ReadFile(filepath.Join(root, "requests", e.Name()))
				var req ipc.CapabilityRequest
				if json.Unmarshal(data, &req) == nil && req.RequestID != "" {
					reqID = req.RequestID
				}
			}
			time.Sleep(5 * time.Millisecond)
		}
		_ = ipc.WriteResponse(root, ipc.CapabilityResponse{
			RequestID: reqID,
			Result:    map[string]any{"echo": "pong"},
		})
	}()

	result, err := h.dispatchCapability("noop", "noop.ping", map[string]any{"msg": "ping"}, 5*time.Second)
	require.NoError(t, err)
	m, ok := result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "pong", m["echo"])
}

func TestDispatchCapability_Timeout(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "requests"), 0700))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "responses"), 0700))
	h := &toolHandlers{ipcDir: root, chatID: "chat-1"}

	_, err := h.dispatchCapability("noop", "noop.ping", map[string]any{}, 200*time.Millisecond)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
}
