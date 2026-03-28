package storage

import (
	"fmt"
	"os"
	"path/filepath"
)

func DefaultSocketPath() string {
	if path := os.Getenv("FOCUS_SOCKET_PATH"); path != "" {
		return path
	}
	return resolveSocketPath(os.Getenv("XDG_RUNTIME_DIR"), os.Getuid())
}

func resolveSocketPath(xdgRuntimeDir string, uid int) string {
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
