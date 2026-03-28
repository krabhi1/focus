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
	printCommandCheck("notify-send", true)
	printCommandCheck("systemctl", false)
	printBackendCheck("lock.backend", detectLockBackend())
	printBackendCheck("unlock.backend", detectUnlockBackend())
	printBackendCheck("sound.backend", detectSoundBackend())
	fmt.Println("required: focus-events, notify-send")
	fmt.Println("optional: lock/unlock and sound backends vary by desktop/session")

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

func printBackendCheck(label, backend string) {
	if backend == "missing" {
		fmt.Printf("%s: missing\n", label)
		return
	}
	if path, err := exec.LookPath(backend); err == nil {
		fmt.Printf("%s: %s (%s)\n", label, backend, path)
		return
	}
	fmt.Printf("%s: %s\n", label, backend)
}

func detectLockBackend() string {
	if sessionID := strings.TrimSpace(os.Getenv("XDG_SESSION_ID")); sessionID != "" {
		if _, err := exec.LookPath("loginctl"); err == nil {
			return "loginctl"
		}
	}
	if _, err := exec.LookPath("xdg-screensaver"); err == nil {
		return "xdg-screensaver"
	}
	if _, err := exec.LookPath("cinnamon-screensaver-command"); err == nil {
		return "cinnamon-screensaver-command"
	}
	if _, err := exec.LookPath("gnome-screensaver-command"); err == nil {
		return "gnome-screensaver-command"
	}
	return "missing"
}

func detectUnlockBackend() string {
	if sessionID := strings.TrimSpace(os.Getenv("XDG_SESSION_ID")); sessionID != "" {
		if _, err := exec.LookPath("loginctl"); err == nil {
			return "loginctl"
		}
	}
	if _, err := exec.LookPath("cinnamon-screensaver-command"); err == nil {
		return "cinnamon-screensaver-command"
	}
	if _, err := exec.LookPath("gnome-screensaver-command"); err == nil {
		return "gnome-screensaver-command"
	}
	return "missing"
}

func detectSoundBackend() string {
	for _, name := range []string{"paplay", "pw-play", "aplay", "mpv", "ffplay", "cvlc", "mpg123"} {
		if _, err := exec.LookPath(name); err == nil {
			return name
		}
	}
	return "missing"
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
