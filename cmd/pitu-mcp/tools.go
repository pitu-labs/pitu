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

type toolHandlers struct {
	ipcDir string
	chatID string
}

func (h *toolHandlers) handleSendMessage(text, sender string) (string, error) {
	msg := ipc.OutboundMessage{ChatID: h.chatID, Text: text, Type: "message"}
	if err := h.writeIPC("messages", msg); err != nil {
		return "", err
	}
	return `{"ok":true}`, nil
}

func (h *toolHandlers) handleScheduleTask(name, schedule, prompt string) (string, error) {
	id := uuid.NewString()
	tf := ipc.TaskFile{Action: "create", ID: id, Name: name, Schedule: schedule, Prompt: prompt, ChatID: h.chatID}
	if err := h.writeIPC("tasks", tf); err != nil {
		return "", err
	}
	return fmt.Sprintf(`{"id":%q}`, id), nil
}

func (h *toolHandlers) handleListTasks() (string, error) {
	// Note: due to the IPC design, listTasks is fire-and-forget from the agent's
	// perspective — the host processes the file asynchronously. The agent will not
	// receive the task list back in this call. If synchronous task listing is needed
	// in future, consider a response-file mechanism or a direct read from the
	// /workspace/memory/ directory where the host could write a tasks snapshot.
	tf := ipc.TaskFile{Action: "list", ChatID: h.chatID}
	return `{"ok":true}`, h.writeIPC("tasks", tf)
}

func (h *toolHandlers) handlePauseTask(id string) (string, error) {
	tf := ipc.TaskFile{Action: "pause", ID: id, ChatID: h.chatID}
	return `{"ok":true}`, h.writeIPC("tasks", tf)
}

func (h *toolHandlers) handleRegisterGroup(name, description string) (string, error) {
	gf := ipc.GroupFile{Name: name, Description: description, ChatID: h.chatID}
	return `{"ok":true}`, h.writeIPC("groups", gf)
}

func (h *toolHandlers) writeIPC(subdir string, v any) error {
	dir := filepath.Join(h.ipcDir, subdir)
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("pitu-mcp: marshal: %w", err)
	}
	name := fmt.Sprintf("%d.json", time.Now().UnixNano())
	return os.WriteFile(filepath.Join(dir, name), data, 0644)
}
