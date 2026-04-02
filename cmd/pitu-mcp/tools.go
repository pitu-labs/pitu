package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/pitu-dev/pitu/internal/ipc"
)

type toolHandlers struct {
	ipcDir     string
	chatID     string
	role       string
	subAgentID string
}

func (h *toolHandlers) handleSendMessage(text, sender string) (string, error) {
	msg := ipc.OutboundMessage{
		ChatID:     h.chatID,
		Text:       text,
		Type:       "message",
		Role:       h.role,
		SubAgentID: h.subAgentID,
	}
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

func (h *toolHandlers) handleSpawnAgent(role, prompt string) (string, error) {
	id := uuid.NewString()
	af := ipc.AgentFile{Action: "spawn", SubAgentID: id, Role: role, Prompt: prompt, ChatID: h.chatID}
	if err := h.writeIPC("agents", af); err != nil {
		return "", err
	}
	return fmt.Sprintf(`{"subAgentId":%q}`, id), nil
}

func (h *toolHandlers) handleReactToMessage(messageID, emoji string) (string, error) {
	mid, err := strconv.Atoi(messageID)
	if err != nil {
		return "", fmt.Errorf("pitu-mcp: reactToMessage: invalid message_id %q: %w", messageID, err)
	}
	rf := ipc.ReactionFile{ChatID: h.chatID, MessageID: mid, Emoji: emoji}
	if err := h.writeIPC("reactions", rf); err != nil {
		return "", err
	}
	return `{"ok":true}`, nil
}

func (h *toolHandlers) writeIPC(subdir string, v any) error {
	dir := filepath.Join(h.ipcDir, subdir)
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("pitu-mcp: marshal: %w", err)
	}
	// Write to a temp file in the parent (non-watched) ipcDir, then rename
	// atomically into the watched subdir. os.Rename fires a single IN_MOVED_TO
	// event (mapped to fsnotify.Create) with the file already complete, avoiding
	// the spurious second inotify event that os.WriteFile produces.
	tmp, err := os.CreateTemp(h.ipcDir, ".tmp-")
	if err != nil {
		return fmt.Errorf("pitu-mcp: create temp: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("pitu-mcp: write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("pitu-mcp: close temp: %w", err)
	}
	name := fmt.Sprintf("%d.json", time.Now().UnixNano())
	if err := os.Rename(tmpPath, filepath.Join(dir, name)); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("pitu-mcp: rename: %w", err)
	}
	return nil
}
