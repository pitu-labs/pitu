package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"

	"github.com/pitu-dev/pitu/internal/store"
)

// runCapabilities is the CLI entry point for `pitu capabilities ...`.
// It opens the store at the default DB path and delegates to runCapabilitiesWithStore.
func runCapabilities(args []string) {
	home, _ := os.UserHomeDir()
	dbPath := filepath.Join(home, ".pitu", "pitu.db")
	st, err := store.New(dbPath)
	if err != nil {
		log.Fatalf("pitu: capabilities: open store: %v", err)
	}
	defer st.Close()
	if err := runCapabilitiesWithStore(st, args); err != nil {
		log.Fatalf("pitu: capabilities: %v", err)
	}
}

// runCapabilitiesWithStore implements the subcommand against an injected store
// (separated for testability).
func runCapabilitiesWithStore(st *store.Store, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: pitu capabilities {list|enable|disable} [--chat <id>] [--capability <name>]")
	}
	action := args[0]
	flags := parseFlags(args[1:])

	switch action {
	case "list":
		all, err := st.ListCapabilitiesByChat()
		if err != nil {
			return err
		}
		if len(all) == 0 {
			fmt.Println("No capabilities enabled for any chat.")
			return nil
		}
		chats := make([]string, 0, len(all))
		for c := range all {
			chats = append(chats, c)
		}
		sort.Strings(chats)
		for _, c := range chats {
			fmt.Printf("Chat %s: %v\n", c, all[c])
		}
		return nil
	case "enable", "disable":
		chat := flags["chat"]
		capability := flags["capability"]
		if chat == "" || capability == "" {
			return fmt.Errorf("%s requires --chat <id> and --capability <name>", action)
		}
		if action == "enable" {
			if err := st.EnableCapability(chat, capability); err != nil {
				return err
			}
			fmt.Printf("Enabled %s for chat %s\n", capability, chat)
		} else {
			if err := st.DisableCapability(chat, capability); err != nil {
				return err
			}
			fmt.Printf("Disabled %s for chat %s\n", capability, chat)
		}
		return nil
	default:
		return fmt.Errorf("unknown action %q (want list|enable|disable)", action)
	}
}

// parseFlags parses a minimal "--key value" sequence into a map.
func parseFlags(args []string) map[string]string {
	out := map[string]string{}
	for i := 0; i+1 < len(args); i += 2 {
		key := args[i]
		if len(key) > 2 && key[:2] == "--" {
			out[key[2:]] = args[i+1]
		}
	}
	return out
}
