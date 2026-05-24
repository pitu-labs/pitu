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

func TestHandleNoopCapability_WritesEchoResponse(t *testing.T) {
	dataDir := t.TempDir()
	chatID := "chat-1"
	require.NoError(t, os.MkdirAll(filepath.Join(dataDir, chatID, "ipc", "responses"), 0700))

	req := ipc.CapabilityRequest{
		RequestID:  "r1",
		Capability: "noop",
		Tool:       "noop.ping",
		Params:     map[string]any{"msg": "hello"},
		ChatID:     chatID,
	}
	handleCapabilityRequest(dataDir, req)

	// Handler runs in a goroutine; poll for the response file.
	path := filepath.Join(dataDir, chatID, "ipc", "responses", "r1.json")
	var data []byte
	var err error
	for i := 0; i < 100; i++ {
		data, err = os.ReadFile(path)
		if err == nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	require.NoError(t, err)

	var resp ipc.CapabilityResponse
	require.NoError(t, json.Unmarshal(data, &resp))
	assert.Equal(t, "r1", resp.RequestID)
	m, ok := resp.Result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "hello", m["echo"])
}

func TestHandleCapability_UnknownCapability_Errors(t *testing.T) {
	dataDir := t.TempDir()
	chatID := "chat-2"
	require.NoError(t, os.MkdirAll(filepath.Join(dataDir, chatID, "ipc", "responses"), 0700))

	handleCapabilityRequest(dataDir, ipc.CapabilityRequest{
		RequestID:  "r2",
		Capability: "bogus",
		ChatID:     chatID,
	})

	path := filepath.Join(dataDir, chatID, "ipc", "responses", "r2.json")
	var data []byte
	var err error
	for i := 0; i < 100; i++ {
		data, err = os.ReadFile(path)
		if err == nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	require.NoError(t, err)

	var resp ipc.CapabilityResponse
	require.NoError(t, json.Unmarshal(data, &resp))
	assert.Equal(t, "capability_disabled", resp.ErrorCode)
	assert.NotEmpty(t, resp.Error)
}
