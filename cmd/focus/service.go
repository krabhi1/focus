package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func userServicePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}

	return filepath.Join(home, ".config", "systemd", "user", "focusd.service"), nil
}

func renderUserService(bindir string) string {
	return fmt.Sprintf(`[Unit]
Description=Focus daemon
After=graphical-session.target
PartOf=graphical-session.target

[Service]
Type=simple
ExecStart=%s/focusd
WorkingDirectory=%%h
Environment=PATH=%s:/usr/local/bin:/usr/bin:/bin
Restart=on-failure
RestartSec=2
NoNewPrivileges=true

[Install]
WantedBy=default.target
`, bindir, bindir)
}
