package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"focus/internal/protocol"
	"focus/internal/storage"
)

func runDoctor() {
	fmt.Println("Focus Doctor")
	fmt.Println("")

	printCommandCheck("focus", true)
	printCommandCheck("focusd", true)
	printCommandCheck("focus-events", true)
	printCommandCheck("xdg-screensaver", true)
	printCommandCheck("notify-send", true)
	printCommandCheck("paplay", false)
	printCommandCheck("systemctl", false)

	socketPath := storage.DefaultSocketPath()
	fmt.Printf("socket.path: %s\n", socketPath)
	if info, err := os.Stat(socketPath); err == nil {
		if info.Mode()&os.ModeSocket != 0 {
			fmt.Println("socket.file: ok")
		} else {
			fmt.Printf("socket.file: unexpected file type (%s)\n", info.Mode().String())
		}
	} else {
		fmt.Printf("socket.file: missing (%v)\n", err)
	}

	conn, err := connectDaemon()
	if err != nil {
		fmt.Printf("daemon.ipc: down (%v)\n", err)
	} else {
		_, reqErr := SendRequest(conn, protocol.Request{Command: "status"})
		_ = conn.Close()
		if reqErr != nil {
			fmt.Printf("daemon.ipc: down (%v)\n", reqErr)
		} else {
			fmt.Println("daemon.ipc: up")
		}
	}

	printSystemdStatus()
}

func printCommandCheck(name string, required bool) {
	if path, err := exec.LookPath(name); err == nil {
		fmt.Printf("dep.%s: ok (%s)\n", name, path)
		return
	}
	if required {
		fmt.Printf("dep.%s: missing (required)\n", name)
	} else {
		fmt.Printf("dep.%s: missing (optional)\n", name)
	}
}

func printSystemdStatus() {
	if _, err := exec.LookPath("systemctl"); err != nil {
		fmt.Println("service.systemd: unavailable (systemctl missing)")
		return
	}

	enabledOut, enabledErr := exec.Command("systemctl", "--user", "is-enabled", "focusd.service").CombinedOutput()
	activeOut, activeErr := exec.Command("systemctl", "--user", "is-active", "focusd.service").CombinedOutput()

	enabled := strings.TrimSpace(string(enabledOut))
	active := strings.TrimSpace(string(activeOut))

	if enabled == "" {
		enabled = "unknown"
	}
	if active == "" {
		active = "unknown"
	}

	fmt.Printf("service.enabled: %s\n", enabled)
	fmt.Printf("service.active: %s\n", active)

	if enabledErr != nil && enabled == "unknown" {
		fmt.Printf("service.enabled.err: %v\n", enabledErr)
	}
	if activeErr != nil && active == "unknown" {
		fmt.Printf("service.active.err: %v\n", activeErr)
	}
}
