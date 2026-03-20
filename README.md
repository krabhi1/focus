# Focus

Focus is a productivity tool designed to keep your computer usage intentional. It helps you stay on track and manage your time effectively, ensuring every session at your desk has a clear purpose rather than drifting into aimless browsing.

The system operates via a background daemon (`focusd`) that handles time tracking, while a simple CLI (`focus`) provides the interface for managing your work.

## Rules

- **Intentional Use:** Focus gives you a 5-minute grace period to use your computer without an active task. Beyond that, the system will warn you before locking the screen or putting the computer to sleep.
- **One Thing at a Time:** Only one task can be active at once. When a task finishes, the daemon enforces a cooldown before you can start the next one. Cooldown scales by task length.
- **Work Modes:**
  - **short (15 min):** No in-task break.
  - **medium (30 min):** No in-task break.
  - **long (60 min):** Break starts at 25 minutes for 5 minutes.
  - **deep (90 min):** Break starts at 45 minutes for 10 minutes.
- **Break Enforcement (long/deep only):**
  - You get a reminder 2 minutes before the break starts.
  - At break start, Focus locks the screen.
  - If the screen is unlocked during break, Focus warns once and relocks after 30 seconds (if still in break).
- **Post-Task Cooldown:** Cooldown starts only after task completion (not during the in-task break).
- **Automatic Tracking:** There is no need for manual toggling. Focus monitors your keyboard, mouse, and screen activity to automatically pause and resume tasks whenever you step away.
- **Heads-up Notifications:** You’ll receive a notification 5 minutes before a task ends, giving you time to wrap up your work gracefully.
- **Status Visibility:** `focus status` shows cooldown state, active task remaining time, and break-specific details including relock countdown when applicable.

## System Requirements

For now this is only available in Linux and tested with cinnamon desktop environment. It should work in other environments as well, but I haven't tested it yet.

## Run

Build everything with:

```bash
make build
```

Run the daemon and client from the built binaries:

```bash
./dist/focusd
./dist/focus status
./dist/focus history
```

Avoid running `go run cmd/daemon/main.go` directly. Use the package path instead if you want to run from source:

```bash
go run ./cmd/daemon
go run ./cmd/client status
```

## Install (user systemd service)

Install latest release (GitHub):

```bash
curl -fsSL https://raw.githubusercontent.com/krabhi1/focus/main/install.sh | sh
```

Install a specific version:

```bash
curl -fsSL https://raw.githubusercontent.com/krabhi1/focus/main/install.sh | sh -s -- --version v0.1.0
```

Manual (recommended for audit): download `install.sh`, review it, then run it.

Install from local source checkout:

Build/install binaries and enable a user-level systemd service:

```bash
./scripts/install.sh
```

This installs:

- `focus`, `focusd`, and `focus-events` to `~/.local/bin` (by default)
- `focusd.service` to `~/.config/systemd/user/focusd.service`

Manage service manually if needed:

```bash
systemctl --user daemon-reload
systemctl --user enable --now focusd.service
systemctl --user status focusd.service
```

Uninstall:

```bash
./scripts/uninstall.sh
```

Optional install flags:

```bash
./scripts/install.sh --prefix /custom/prefix
./scripts/install.sh --no-build
./scripts/install.sh --no-systemd
```

Current prebuilt release target: `linux/amd64`.
