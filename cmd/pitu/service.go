package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/pitu-dev/pitu/internal/service"
)

// runService is the entry point for `pitu service <subcommand>`.
func runService(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: pitu service <install|uninstall|start|stop|status|logs [-n N]>")
		os.Exit(1)
	}

	mgr, err := service.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, "pitu service:", err)
		os.Exit(1)
	}

	switch args[0] {
	case "install":
		err = mgr.Install()
	case "uninstall":
		err = mgr.Uninstall()
	case "start":
		err = mgr.Start()
	case "stop":
		err = mgr.Stop()
	case "status":
		var state string
		state, err = mgr.Status()
		if err == nil {
			fmt.Println("pitu service:", state)
		}
	case "logs":
		fs := flag.NewFlagSet("logs", flag.ExitOnError)
		n := fs.Int("n", 50, "number of recent lines to show before following")
		fs.Parse(args[1:])
		err = mgr.Logs(*n)
	default:
		fmt.Fprintf(os.Stderr, "pitu service: unknown subcommand %q\n", args[0])
		fmt.Fprintln(os.Stderr, "usage: pitu service <install|uninstall|start|stop|status|logs [-n N]>")
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "pitu service:", err)
		os.Exit(1)
	}
}
