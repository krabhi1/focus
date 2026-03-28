package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"focus/internal/effects"
	"focus/internal/protocol"
	"focus/internal/storage"
)

func runDoctor() {
	fmt.Println(colorTitle("Focus Doctor"))
	fmt.Println("")

	printCommandCheck("focus", true)
	printCommandCheck("focusd", true)
	printCommandCheck("focus-events", true)
	printCommandCheck("notify-send", true)
	printCommandCheck("systemctl", false)
	printBackendCheck("desktop", effects.DetectDesktopFlavor())
	printBackendCheck("lock.backend", effects.DetectLockBackend())
	printBackendCheck("unlock.backend", effects.DetectUnlockBackend())
	printBackendCheck("sound.backend", effects.DetectSoundBackend())
	fmt.Println(colorHeading("required:"), colorSuccess("focus-events, notify-send"))
	fmt.Println(colorHeading("optional:"), colorMuted("lock/unlock and sound backends vary by desktop/session"))

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
		fmt.Printf("%s %s (%s)\n", colorInfo("dep."+name+":"), colorSuccess("ok"), path)
		return
	}
	if required {
		fmt.Printf("%s %s\n", colorInfo("dep."+name+":"), colorError("missing (required)"))
	} else {
		fmt.Printf("%s %s\n", colorInfo("dep."+name+":"), colorWarn("missing (optional)"))
	}
}

func printBackendCheck(label, backend string) {
	if backend == "missing" {
		fmt.Printf("%s %s\n", colorInfo(label+":"), colorWarn("missing"))
		return
	}
	if path, err := exec.LookPath(backend); err == nil {
		fmt.Printf("%s %s (%s)\n", colorInfo(label+":"), colorSuccess(backend), path)
		return
	}
	fmt.Printf("%s %s\n", colorInfo(label+":"), colorMuted(backend))
}

func detectLockBackend() string   { return effects.DetectLockBackend() }
func detectUnlockBackend() string { return effects.DetectUnlockBackend() }
func detectSoundBackend() string  { return effects.DetectSoundBackend() }

func printSystemdStatus() {
	if _, err := exec.LookPath("systemctl"); err != nil {
		fmt.Println(colorInfo("service.systemd:"), colorWarn("unavailable (systemctl missing)"))
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

	fmt.Printf("%s %s\n", colorInfo("service.enabled:"), colorSuccess(enabled))
	fmt.Printf("%s %s\n", colorInfo("service.active:"), colorSuccess(active))

	if enabledErr != nil && enabled == "unknown" {
		fmt.Printf("%s %s\n", colorInfo("service.enabled.err:"), colorWarn(enabledErr.Error()))
	}
	if activeErr != nil && active == "unknown" {
		fmt.Printf("%s %s\n", colorInfo("service.active.err:"), colorWarn(activeErr.Error()))
	}
}
