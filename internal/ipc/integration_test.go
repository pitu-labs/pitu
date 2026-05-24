package ipc_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pitu-dev/pitu/internal/ipc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Simulates the full harness path: a request file dropped into requests/ is routed,
// an echo handler writes a response, and we observe the response file. Proves the
// primitive works end-to-end without pitu-mcp or a container.
func TestIntegration_RequestRoutedAndResponded(t *testing.T) {
	root := t.TempDir()

	router := ipc.NewRouter(
		func(ipc.OutboundMessage) {}, func(ipc.TaskFile) {}, func(ipc.GroupFile) {},
		func(ipc.AgentFile) {}, func(ipc.ReactionFile) {},
	)
	router.SetCapabilityHandler(func(c ipc.CapabilityRequest) {
		// echo handler (runs in goroutine like the real harness)
		go func() {
			_ = ipc.WriteResponse(root, ipc.CapabilityResponse{
				RequestID: c.RequestID,
				Result:    map[string]any{"echo": c.Params["msg"]},
			})
		}()
	})

	w, err := ipc.NewWatcher(router)
	require.NoError(t, err)
	require.NoError(t, w.RegisterDir(root, "chat-1", "", ""))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Watch(ctx)
	time.Sleep(50 * time.Millisecond) // let watcher start

	// Drop a request via atomic rename (mirrors pitu-mcp writeIPC).
	body := `{"request_id":"int-1","capability":"noop","tool":"noop.ping","params":{"msg":"yo"},"chat_id":"x"}`
	tmp := filepath.Join(root, ".tmp-int")
	require.NoError(t, os.WriteFile(tmp, []byte(body), 0600))
	require.NoError(t, os.Rename(tmp, filepath.Join(root, "requests", "int-1.json")))

	// Wait for the response file.
	respPath := filepath.Join(root, "responses", "int-1.json")
	var data []byte
	for i := 0; i < 200; i++ {
		data, err = os.ReadFile(respPath)
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.NoError(t, err, "response file never appeared")

	var resp ipc.CapabilityResponse
	require.NoError(t, json.Unmarshal(data, &resp))
	m := resp.Result.(map[string]any)
	assert.Equal(t, "yo", m["echo"])
}
