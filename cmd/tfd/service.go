package main

// Installing this daemon as the machine's service.
//
// The flags after `install` are the flags the service will run with, which is
// what makes this one command instead of a unit file to edit: whatever relay
// and device this machine was set up with is what it dials after a reboot.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/CarlOpenLab/term-fleet/internal/service"
)

func serviceCommand(args []string) {
	const usage = `usage:
  tfd service install [flags...]   install and start this daemon as a service
  tfd service uninstall            stop and remove the service
  tfd service restart              restart it (its sessions end with it)
  tfd service status               print what the service manager says

The flags are the ones the service will run with, for example:
  tfd service install -relay wss://relay.example/relay/agent \
      -device-id mac-mini-1a2b3c -device-secret "$SECRET"
`
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	switch args[0] {
	case "install":
		bin, err := os.Executable()
		if err != nil {
			fail("cannot find this binary: %v", err)
		}
		if resolved, err := filepath.EvalSymlinks(bin); err == nil {
			bin = resolved
		}
		msg, err := service.Install(service.Config{Bin: bin, Args: args[1:]})
		if err != nil {
			fail("%v", err)
		}
		fmt.Println(msg)
		fmt.Println("binary:", bin)
		if len(args) == 1 {
			fmt.Println("no flags given: the service runs with this daemon's defaults (LAN only, no relay)")
		}
		fmt.Println("the service file holds this machine's credential and is mode 0600; keep it private")
	case "uninstall":
		if err := service.Uninstall(); err != nil {
			fail("%v", err)
		}
		fmt.Println("removed")
	case "restart":
		if err := service.Restart(); err != nil {
			fail("%v", err)
		}
		fmt.Println("restarted; its sessions ended with the old process")
	case "status":
		state, err := service.Status()
		if err != nil {
			fmt.Printf("%s (%v)\n", state, err)
			return
		}
		fmt.Println(strings.TrimSpace(state))
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "tfd service: "+format+"\n", args...)
	os.Exit(1)
}
