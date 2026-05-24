package ipc

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCapabilityRequest_RoundTrip(t *testing.T) {
	in := CapabilityRequest{
		RequestID:  "req-1",
		Capability: "noop",
		Tool:       "noop.ping",
		Params:     map[string]any{"msg": "hi"},
		ChatID:     "chat-9",
	}
	data, err := json.Marshal(in)
	require.NoError(t, err)

	var out CapabilityRequest
	require.NoError(t, json.Unmarshal(data, &out))
	assert.Equal(t, "req-1", out.RequestID)
	assert.Equal(t, "noop", out.Capability)
	assert.Equal(t, "noop.ping", out.Tool)
	assert.Equal(t, "hi", out.Params["msg"])
	assert.Equal(t, "chat-9", out.ChatID)
}

func TestCapabilityResponse_RoundTrip(t *testing.T) {
	in := CapabilityResponse{RequestID: "req-1", Result: map[string]any{"ok": true}}
	data, err := json.Marshal(in)
	require.NoError(t, err)

	var out CapabilityResponse
	require.NoError(t, json.Unmarshal(data, &out))
	assert.Equal(t, "req-1", out.RequestID)
	assert.Empty(t, out.Error)
}

func TestWriteResponse_AtomicNamedByRequestID(t *testing.T) {
	root := t.TempDir()
	resp := CapabilityResponse{RequestID: "abc-123", Result: map[string]any{"ok": true}}

	require.NoError(t, WriteResponse(root, resp))

	path := filepath.Join(root, "responses", "abc-123.json")
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var got CapabilityResponse
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, "abc-123", got.RequestID)

	// No leftover temp files in the ipc root.
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		assert.NotContains(t, e.Name(), ".tmp-")
	}
}
