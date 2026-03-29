package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/pitu-dev/pitu/internal/config"
	"github.com/pitu-dev/pitu/internal/container"
	"github.com/pitu-dev/pitu/internal/ipc"
	"github.com/pitu-dev/pitu/internal/queue"
	"github.com/pitu-dev/pitu/internal/scheduler"
	"github.com/pitu-dev/pitu/internal/skills"
	"github.com/pitu-dev/pitu/internal/store"
	"github.com/pitu-dev/pitu/internal/telegram"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "service" {
		runService(os.Args[2:])
		return
	}
	cfgPath := os.Getenv("PITU_CONFIG")
	if cfgPath == "" {
		home, _ := os.UserHomeDir()
		cfgPath = filepath.Join(home, ".pitu", "config.toml")
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("pitu: config: %v", err)
	}

	dbPath := cfg.DB.Path
	if dbPath == "" {
		home, _ := os.UserHomeDir()
		dbPath = filepath.Join(home, ".pitu", "pitu.db")
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0700); err != nil {
		log.Fatalf("pitu: mkdir db: %v", err)
	}
	st, err := store.New(dbPath)
	if err != nil {
		log.Fatalf("pitu: store: %v", err)
	}
	defer st.Close()

	// Skills discovery
	home, _ := os.UserHomeDir()
	skillsPaths := append([]string{
		filepath.Join(home, ".agents", "skills"),
		filepath.Join(home, ".pitu", "skills"),
		".agents/skills",
		".pitu/skills",
	}, cfg.Skills.ExtraPaths...)
	discovered := skills.Discover(skillsPaths)
	log.Printf("pitu: discovered %d skills", len(discovered))

	agentDir := filepath.Join(home, ".pitu", "agent")
	agentCfg := skills.LoadAgentConfig(agentDir)

	dataDir := filepath.Join(home, ".pitu", "data")
	os.MkdirAll(dataDir, 0700)

	// Merge all discovered skills into a single directory for container mounting
	skillsMount := mergeSkills(dataDir, discovered)

	// Declare q, mgr, ctx, and cancel before any closures that reference them (Go requires declaration before use in closures).
	var q *queue.Queue
	var mgr *container.Manager
	var ctx context.Context
	var cancel context.CancelFunc

	// Telegram
	sender := telegram.NewSender(cfg.Telegram.BotToken, "https://api.telegram.org")
	poller := telegram.NewPoller(cfg.Telegram.BotToken, "https://api.telegram.org")

	// IPC router — wires outbound container files to Telegram/scheduler/store
	sched := scheduler.New(func(chatID, prompt string) {
		msg := ipc.InboundMessage{ChatID: chatID, Text: prompt, From: "scheduler", MessageID: fmt.Sprintf("sched-%d", time.Now().UnixNano())}
		q.Enqueue(chatID, func() {
			mgr.Dispatch(context.Background(), chatID, msg)
		})
	})

	router := ipc.NewRouter(
		func(m ipc.OutboundMessage) {
			if m.SubAgentID != "" {
				// Bubble up!
				bubbleMsg := ipc.InboundMessage{
					ChatID: m.ChatID,
					From:   fmt.Sprintf("Agent: %s", m.Role),
					Text:   fmt.Sprintf("[Agent: %s (%s)] %s", m.Role, m.SubAgentID, m.Text),
				}
				q.Enqueue(m.ChatID, func() {
					mgr.Dispatch(context.Background(), m.ChatID, bubbleMsg)
				})
				return
			}
			// Standard Telegram delivery
			if err := sender.SendMessage(m.ChatID, m.Text); err != nil {
				log.Printf("pitu: send message: %v", err)
			}
		},
		func(tf ipc.TaskFile) {
			switch tf.Action {
			case "create":
				t := store.Task{ID: tf.ID, ChatID: tf.ChatID, Name: tf.Name, Schedule: tf.Schedule, Prompt: tf.Prompt}
				st.SaveTask(t)
				sched.Add(t)
			case "pause":
				st.PauseTask(tf.ID)
				sched.Pause(tf.ID)
			}
		},
		func(gf ipc.GroupFile) {
			st.RegisterGroup(gf.ChatID, gf.Name, gf.Description)
		},
		func(af ipc.AgentFile) {
			if af.Action != "spawn" {
				log.Printf("pitu: onAgent: unknown action %q, ignoring", af.Action)
				return
			}
			// ctx is assigned before w.Watch starts (line ~143), so it is never nil here.
			mgr.SpawnSubAgent(ctx, af.ChatID, af.Role, af.Prompt)
		},
	)

	// IPC watcher — dynamically registers new container dirs as they start
	w, err := ipc.NewWatcher(router)
	if err != nil {
		log.Fatalf("pitu: ipc watcher: %v", err)
	}

	// Container manager — warm pool; calls w.RegisterDir on each new container
	mgr = container.NewManager(cfg, discovered, w, nil)
	mgr.SetDirs(dataDir, skillsMount)

	// Queue — per-chat FIFO, global concurrency cap
	q = queue.New(cfg.Container.MaxConcurrent)

	// Load persisted tasks into scheduler
	activeTasks, _ := st.GetAllActiveTasks()
	for _, t := range activeTasks {
		sched.Add(t)
	}

	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()

	// Graceful shutdown on SIGINT/SIGTERM
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("pitu: shutting down...")
		cancel()
		mgr.StopAll()
		q.Stop()
	}()

	// Start background goroutines
	go w.Watch(ctx)
	go sched.Run(ctx)

	log.Println("pitu: started, polling Telegram...")
	poller.Poll(ctx, func(u telegram.Update) {
		if u.Message.Text == "" {
			return
		}
		chatID := fmt.Sprintf("%d", u.Message.Chat.ID)
		msg := ipc.InboundMessage{
			ChatID:    chatID,
			From:      u.Message.From.FirstName,
			Text:      u.Message.Text,
			MessageID: fmt.Sprintf("%d", u.Message.MessageID),
		}
		st.SaveMessage(store.Message{
			ChatID:    chatID,
			FromUser:  u.Message.From.FirstName,
			Text:      u.Message.Text,
			MessageID: fmt.Sprintf("%d", u.Message.MessageID),
			CreatedAt: time.Now().UTC(),
		})
		// Write CONTEXT.md on first boot
		memDir := filepath.Join(dataDir, chatID, "memory")
		os.MkdirAll(memDir, 0700)
		skills.WriteContext(memDir, chatID, discovered, agentCfg)

		q.Enqueue(chatID, func() {
			if err := mgr.Dispatch(ctx, chatID, msg); err != nil {
				log.Printf("pitu: dispatch chat %s: %v", chatID, err)
			}
		})
	})
}

// mergeSkills copies all discovered skills into a single scratch directory
// (dataDir/skills/) so containers see a unified merged view via one mount.
// Project-level skills (higher precedence) are copied last and overwrite user-level
// copies with the same name — matching the precedence order of skills.Discover.
func mergeSkills(dataDir string, discovered []skills.Skill) string {
	mergedDir := filepath.Join(dataDir, "skills")
	os.MkdirAll(mergedDir, 0700)
	// Copy in reverse order so higher-precedence entries (index 0) win
	for i := len(discovered) - 1; i >= 0; i-- {
		s := discovered[i]
		skillSrcDir := filepath.Dir(s.Path) // parent of SKILL.md
		dest := filepath.Join(mergedDir, s.Name)
		os.MkdirAll(dest, 0700)
		// Copy entire skill directory tree
		copyDir(skillSrcDir, dest)
	}
	return mergedDir
}

func copyDir(src, dst string) {
	entries, err := os.ReadDir(src)
	if err != nil {
		return
	}
	for _, e := range entries {
		srcPath := filepath.Join(src, e.Name())
		dstPath := filepath.Join(dst, e.Name())
		if e.IsDir() {
			os.MkdirAll(dstPath, 0700)
			copyDir(srcPath, dstPath)
		} else {
			data, err := os.ReadFile(srcPath)
			if err == nil {
				os.WriteFile(dstPath, data, 0600)
			}
		}
	}
}
