package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"focus/internal/effects"
	"focus/internal/protocol"
	"focus/internal/storage"
)

func runDoctor(all bool) {
	fmt.Println(colorTitle("Focus Doctor"))
	fmt.Println("")

	printCommandCheck("focus", true)
	printInstalledCommandCheck("focusd", true)
	printInstalledCommandCheck("focus-events", true)
	printCommandCheck("notify-send", true)
	printCommandCheck("systemctl", false)
	printBackendCheck("desktop", effects.DetectDesktopFlavor())
	printBackendCheck("lock.backend", effects.DetectLockBackend())
	printBackendCheck("unlock.backend", effects.DetectUnlockBackend())
	printBackendCheck("sleep.backend", effects.DetectSleepBackend())
	printBackendCheck("sound.backend", effects.DetectSoundBackend())
	fmt.Println(colorHeading("required:"), colorSuccess("focus-events, notify-send"))
	fmt.Println(colorHeading("optional:"), colorMuted("lock/unlock/sleep and sound backends vary by desktop/session"))

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
	if all {
		fmt.Println("")
		printRuntimeDebug()
	}
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

func printInstalledCommandCheck(name string, required bool) {
	if path := installedBinaryPath(name); path != "" {
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() && info.Mode()&0o111 != 0 {
			fmt.Printf("%s %s (%s)\n", colorInfo("dep."+name+":"), colorSuccess("ok"), path)
			return
		}
	}
	printCommandCheck(name, required)
}

func installedBinaryPath(name string) string {
	if p := os.Getenv("FOCUS_LIBEXEC_DIR"); p != "" {
		return filepath.Join(p, name)
	}
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	bindir := filepath.Dir(exe)
	prefix := filepath.Dir(bindir)
	return filepath.Join(prefix, "libexec", "focus", name)
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
func detectSleepBackend() string  { return effects.DetectSleepBackend() }
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

func printRuntimeDebug() {
	fmt.Println(colorHeading("runtime debug:"))
	conn, err := connectDaemon()
	if err != nil {
		fmt.Printf("  %s %v\n", colorInfo("daemon.debug:"), colorWarn(err.Error()))
		return
	}
	defer conn.Close()

	res, err := SendRequest(conn, protocol.Request{Command: "debug"})
	if err != nil {
		fmt.Printf("  %s %v\n", colorInfo("daemon.debug:"), colorWarn(err.Error()))
		return
	}
	if res.Error != nil {
		fmt.Printf("  %s %v\n", colorInfo("daemon.debug:"), colorWarn(res.Error.Message))
		return
	}
	if res.Success == nil || strings.TrimSpace(res.Success.Message) == "" {
		fmt.Printf("  %s %s\n", colorInfo("daemon.debug:"), colorWarn("empty"))
		return
	}
	for _, line := range strings.Split(strings.TrimRight(res.Success.Message, "\n"), "\n") {
		fmt.Println("  " + line)
	}
}
