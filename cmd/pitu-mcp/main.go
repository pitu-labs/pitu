package main

import (
	"log"
	"os"

	"github.com/mark3labs/mcp-go/server"
)

const ipcDir = "/workspace/ipc"

func main() {
	chatID := os.Getenv("PITU_CHAT_ID")
	if chatID == "" {
		log.Fatal("pitu-mcp: PITU_CHAT_ID is not set")
	}
	role := os.Getenv("PITU_ROLE")
	subAgentID := os.Getenv("PITU_SUB_AGENT_ID")

	h := &toolHandlers{
		ipcDir:     ipcDir,
		chatID:     chatID,
		role:       role,
		subAgentID: subAgentID,
	}
	s := buildServer(h)

	if err := server.ServeStdio(s); err != nil {
		log.Fatalf("pitu-mcp: %v", err)
	}
}
