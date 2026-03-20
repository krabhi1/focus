package state

import (
	"fmt"
	"os"
	"path/filepath"
)

// SocketPath returns the daemon unix socket path for the current user.
func SocketPath() string {
	return resolveSocketPath(os.Getenv("FOCUS_SOCKET_PATH"), os.Getenv("XDG_RUNTIME_DIR"), os.Getuid())
}

func resolveSocketPath(overridePath string, xdgRuntimeDir string, uid int) string {
	if overridePath != "" {
		return overridePath
	}
	if xdgRuntimeDir != "" {
		return filepath.Join(xdgRuntimeDir, "focus", "focus.sock")
	}
	if uid >= 0 {
		runUser := filepath.Join("/run/user", fmt.Sprintf("%d", uid))
		if info, err := os.Stat(runUser); err == nil && info.IsDir() {
			return filepath.Join(runUser, "focus", "focus.sock")
		}
		return filepath.Join("/tmp", fmt.Sprintf("focus-%d.sock", uid))
	}
	return filepath.Join("/tmp", "focus.sock")
}
