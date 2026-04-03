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

	// Skills discovery — use binary-relative paths so the service finds bundled
	// skills regardless of CWD (systemd/launchd don't set a meaningful working dir).
	home, _ := os.UserHomeDir()
	skillsPaths := []string{
		filepath.Join(home, ".agents", "skills"),
		filepath.Join(home, ".pitu", "skills"),
	}
	if exe, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			binaryDir := filepath.Dir(resolved)
			skillsPaths = append(skillsPaths,
				filepath.Join(binaryDir, ".agents", "skills"),
				filepath.Join(binaryDir, ".pitu", "skills"),
			)
		}
	}
	skillsPaths = append(skillsPaths, cfg.Skills.ExtraPaths...)
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
				// Validate before SaveTask so an invalid schedule is rejected before the DB
				// is touched. Add would also reject it (same parser), but SaveTask would have
				// already persisted the row, requiring a compensating delete.
				if err := sched.Validate(t.Schedule); err != nil {
					log.Printf("pitu: task %s: invalid schedule: %v", t.ID, err)
					return
				}
				if err := st.SaveTask(t); err != nil {
					log.Printf("pitu: task %s: save: %v", t.ID, err)
					return
				}
				if err := sched.Add(t); err != nil {
					log.Printf("pitu: task %s: add to cron: %v", t.ID, err)
					if delErr := st.DeleteTask(t.ID); delErr != nil {
						log.Printf("pitu: task %s: compensating delete failed: %v", t.ID, delErr)
					}
					if err := writeTasksSnapshot(st, dataDir, t.ChatID); err != nil {
						log.Printf("pitu: task %s: snapshot: %v", t.ID, err)
					}
					return
				}
				if err := writeTasksSnapshot(st, dataDir, t.ChatID); err != nil {
					log.Printf("pitu: task %s: snapshot: %v", t.ID, err)
				}
			case "pause":
				if err := st.PauseTask(tf.ID); err != nil {
					log.Printf("pitu: task %s: pause: %v", tf.ID, err)
					return
				}
				sched.Pause(tf.ID)
				if err := writeTasksSnapshot(st, dataDir, tf.ChatID); err != nil {
					log.Printf("pitu: task %s: snapshot: %v", tf.ID, err)
				}
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
		func(rf ipc.ReactionFile) {
			if err := sender.ReactToMessage(rf.ChatID, rf.MessageID, rf.Emoji); err != nil {
				log.Printf("pitu: react to message: %v", err)
			}
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
		if err := sched.Add(t); err != nil {
			log.Printf("pitu: startup: task %s: add to cron: %v", t.ID, err)
		}
	}

	// Write startup snapshots so agents see current task state immediately after restart.
	startupChats := map[string]struct{}{}
	for _, t := range activeTasks {
		startupChats[t.ChatID] = struct{}{}
	}
	for chatID := range startupChats {
		if err := writeTasksSnapshot(st, dataDir, chatID); err != nil {
			log.Printf("pitu: startup: snapshot for chat %s: %v", chatID, err)
		}
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
		if !isAllowed(u.Message.Chat.ID, cfg.Telegram.AllowedChatIDs) {
			log.Printf("pitu: rejected message from chat %d (not in allowlist)", u.Message.Chat.ID)
			return
		}
		if u.Message.Text == "" {
			return
		}
		chatID := fmt.Sprintf("%d", u.Message.Chat.ID)
		if err := sender.SendChatAction(chatID, "typing"); err != nil {
			log.Printf("pitu: sendChatAction: %v", err)
		}
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

// isAllowed reports whether chatID is permitted to use the bot.
// If the allowlist is empty, all callers are permitted (backward-compatible default).
func isAllowed(chatID int64, allowed []int64) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, id := range allowed {
		if id == chatID {
			return true
		}
	}
	return false
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
