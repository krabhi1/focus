# Focus Manual: Config and Commands

This document explains the runtime config and the main commands in one place.
The README keeps only a short summary and links here for the full reference.

## What Runs Where

Focus has two user-facing binaries:

- `focusd`: the background daemon
- `focus`: the CLI client

The daemon owns runtime flow, deadline scheduling, screen lock actions, notifications, and the private `focus-events` helper.
The client sends commands to the daemon over the Unix socket.

Current architecture:

- `internal/app/runtime.go` orchestrates runtime flow and side effects.
- `internal/domain` is the canonical phase reducer.
- `internal/scheduler` executes deadlines.
- `internal/storage` provides config, presets, history persistence, and socket path helpers.

## Runtime Paths

These paths are resolved at runtime.

- Config file: `~/.config/focus/config.json`
- Config override: `FOCUS_CONFIG=/path/to/config.json`
- Socket path: resolved by `storage.DefaultSocketPath()`
- Socket override: `FOCUS_SOCKET_PATH=/path/to/focus.sock`
- History log: `~/.config/focus/history.jsonl`
- History override: `FOCUS_HISTORY_FILE=/path/to/history.jsonl`

The daemon prints the resolved config, socket, and history paths on startup when trace mode is enabled.
It also prints loaded today-history count and effective runtime durations in trace mode.

The `focus` CLI colorizes output automatically in interactive terminals. Set `NO_COLOR=1` or redirect output to keep plain text.

## Configuration

Focus reads JSON config. The file is split into sections so each part of the flow can be tuned separately.

Example:

```json
{
  "task": {
    "short": "15m",
    "medium": "30m",
    "long": "60m",
    "deep": "90m"
  },
  "cooldown": {
    "short": "5m",
    "long": "10m",
    "deep": "15m"
  },
  "relock_delay": "5s",
  "cooldown_start_delay": "2m",
  "break": {
    "long_start": "25m",
    "deep_start": "45m",
    "warning": "2m",
    "long_duration": "5m",
    "deep_duration": "5m"
  },
  "idle": {
    "warn_after": "3m",
    "lock_after": "2m"
  },
  "alert": {
    "repeat_count": 3
  }
}
```

### `task`

Task preset durations used by `focus start --duration ...`.

- `task.short`: duration for `short`
- `task.medium`: duration for `medium`
- `task.long`: duration for `long`
- `task.deep`: duration for `deep`

These must be strictly increasing:

`short < medium < long < deep`

### `cooldown`

Cooldown starts after a task completes.

- `cooldown.short`: cooldown after a short task
- `cooldown.long`: cooldown after a long task
- `cooldown.deep`: cooldown after a deep task

The daemon resolves cooldown from the task duration.

### `relock_delay`

Shared relock delay used when the user unlocks during break or cooldown.

- `relock_delay`: how long to wait before locking again after an unlock during break or cooldown; `0s` means lock immediately

Lock and unlock support are best-effort. Focus picks the first available session/backend command and does nothing if none are present.

### `cooldown_start_delay`

Delay after task completion before cooldown begins.

- `cooldown_start_delay`: how long to wait after task completion before cooldown starts

### `break`

Break settings apply to long and deep tasks.

- `break.long_start`: when a long task enters break
- `break.deep_start`: when a deep task enters break
- `break.warning`: reminder before break starts
- `break.long_duration`: how long the long-task break lasts
- `break.deep_duration`: how long the deep-task break lasts

Important invariants:

- `break.warning < break.long_start`
- `break.warning < break.deep_start`
- `break.long_start < task.long`
- `break.deep_start < task.deep`
- `break.long_start + break.long_duration < task.long`
- `break.deep_start + break.deep_duration < task.deep`
- `relock_delay < break.long_duration`
- `relock_delay < break.deep_duration`

### `idle`

Daemon-side no-task policy.

- `idle.warn_after`: delay after no task is active before warning
- `idle.lock_after`: delay after no task is active before locking

These values control the wall-clock no-task timer. They are not tied to helper idle detection.

### `alert`

Completion alert settings.

- `alert.repeat_count`: how many times the completion sound plays after task completion; `0` disables sound, `1` plays once, higher values repeat every 3 seconds while the screen stays locked

Sound playback is best-effort. Focus tries common headless audio tools and skips sound if none are installed.

## Config Validation

Focus rejects invalid timing combinations at startup.

Examples:

- missing or non-positive durations are rejected
- task presets must be strictly increasing
- break timings must fit inside the task duration
- idle warn must be less than idle lock
- alert repeat count must be non-negative

If config validation fails, the daemon exits before serving requests.

## Daemon Commands

These are the daemon-side runtime options.

### `focusd`

Start the daemon from source:

```bash
go run ./cmd/focusd
```

Enable runtime flow trace logs:

```bash
FOCUS_TRACE_FLOW=1 go run ./cmd/focusd
```

Common flags:

- `--config <path>`: load a config file instead of the default path
- `--task-short <duration>`: override `task.short`
- `--task-medium <duration>`: override `task.medium`
- `--task-long <duration>`: override `task.long`
- `--task-deep <duration>`: override `task.deep`
- `--cooldown-short <duration>`: override `cooldown.short`
- `--cooldown-long <duration>`: override `cooldown.long`
- `--cooldown-deep <duration>`: override `cooldown.deep`
- `--break-long-start <duration>`: override `break.long_start`
- `--break-deep-start <duration>`: override `break.deep_start`
- `--break-warning <duration>`: override `break.warning`
- `--break-long-duration <duration>`: override `break.long_duration`
- `--break-deep-duration <duration>`: override `break.deep_duration`
- `--relock-delay <duration>`: override `relock_delay`
- `--cooldown-start-delay <duration>`: override `cooldown_start_delay`
- `--idle-warn-after <duration>`: override `idle.warn_after`
- `--idle-lock-after <duration>`: override `idle.lock_after`
- `--completion-alert-repeat-count <count>`: override `alert.repeat_count`

Precedence:

1. CLI flag override
2. JSON config file
3. built-in defaults

## Client Commands

These are the commands users run from the `focus` client.

### `focus status`

Shows the current daemon state.

Example:

```bash
focus status
```

Use it to see:

- active task
- cooldown remaining time
- break state
- relock countdown during break

### `focus start`

Starts a new task.

Example:

```bash
focus start --name "write docs" --duration long --no-break
```

Arguments:

- `--name`: task title
- `--duration`: one of `short`, `medium`, `long`, `deep`
- `--no-break`: skip the in-task break for this task

The client sends the preset name. The daemon resolves the actual duration from config.
With `--no-break`, the task still completes and enters cooldown, but it does not schedule the in-task break.

### `focus cancel`

Cancels the active task if it is still cancelable.

Example:

```bash
focus cancel
```

### `focus history`

Shows today’s completed task history from the persisted log.

Example:

```bash
focus history
```

Use `focus history --all` to show every completed task in the persisted history file.

Current behavior:

- only completed tasks are persisted
- only today’s tasks are loaded into memory on startup
- the history file itself is append-only

### `focus reload`

Reloads runtime config from disk.

Example:

```bash
focus reload
```

Use this after editing the config file.

### `focus config`

Reads or updates one config value in the JSON file.

Example:

```bash
focus config idle.lock_after
focus config idle.lock_after 3m
```

Use one argument to read the current value and default. Use two arguments to update the value and reload the daemon.
The key uses dot notation for nested fields. Supported values are duration strings.

### `focus doctor`

Checks local setup and dependencies.

Example:

```bash
focus doctor
```

This is useful for checking:

- binary presence
- daemon socket health
- service setup
- helper dependencies

### `focus version`

Prints the installed version.

Example:

```bash
focus version
```

### `focus update`

Updates the installed binaries.

Examples:

```bash
focus update
focus update --version v0.1.4
focus update --prefix /custom/prefix
focus update --yes
```

Flags:

- `--version`: install a specific release tag
- `--prefix`: use a custom install prefix
- `--yes`: skip confirmation

### `focus uninstall`

Removes the installed binaries and service after a `y/N` confirmation prompt.

Examples:

```bash
focus uninstall
focus uninstall --prefix /custom/prefix
```

## Common Workflows

### Start a task

```bash
focus start --name "write docs" --duration long
```

Use `short`, `medium`, `long`, or `deep` depending on how long you want the session to run.
Add `--no-break` when you want the task to run straight through without the break phase.

### Check current state

```bash
focus status
```

This shows the active task, cooldown state, or break state.

### Reload config

```bash
focus reload
```

Use this after editing `config.json` or changing runtime overrides.

### View today's history

```bash
focus history
```

This reads from the persisted history log and shows the entries loaded for today.

### Run the manual smoke test

```bash
FOCUS_CONFIG=./focus.dev.json \
FOCUS_SOCKET_PATH=/tmp/focus-dev.sock \
FOCUS_HISTORY_FILE=/tmp/focus-history.jsonl \
go run ./cmd/focusd
```

In another terminal:

```bash
FOCUS_SOCKET_PATH=/tmp/focus-dev.sock go run ./cmd/focus start --name demo --duration long
```

See [manual smoke test](smoke-test.md) for the full step-by-step flow.

## Production vs Dev Config

### Production config

Keep the normal production values in `~/.config/focus/config.json`.

Typical values are long enough to match real work sessions:

```json
{
  "task": {
    "short": "15m",
    "medium": "30m",
    "long": "60m",
    "deep": "90m"
  },
  "cooldown": {
    "short": "5m",
    "long": "10m",
    "deep": "15m"
  },
  "relock_delay": "5s",
  "cooldown_start_delay": "2m",
  "break": {
    "long_start": "25m",
    "deep_start": "45m",
    "warning": "2m",
    "long_duration": "5m",
    "deep_duration": "5m"
  },
  "idle": {
    "warn_after": "3m",
    "lock_after": "2m"
  },
  "alert": {
    "repeat_count": 3
  }
}
```

### Dev config

Use a local `focus.dev.json` when you want to test the full flow quickly:

```json
{
  "task": {
    "short": "5s",
    "medium": "10s",
    "long": "20s",
    "deep": "30s"
  },
  "cooldown": {
    "short": "5s",
    "long": "6s",
    "deep": "7s"
  },
  "relock_delay": "2s",
  "cooldown_start_delay": "10s",
  "break": {
    "long_start": "5s",
    "deep_start": "10s",
    "warning": "2s",
    "long_duration": "3s",
    "deep_duration": "4s"
  },
  "idle": {
    "warn_after": "5s",
    "lock_after": "10s"
  },
  "alert": {
    "repeat_count": 3
  }
}
```

Recommended overrides for dev runs:

```bash
FOCUS_CONFIG=./focus.dev.json \
FOCUS_SOCKET_PATH=/tmp/focus-dev.sock \
FOCUS_HISTORY_FILE=/tmp/focus-history.jsonl \
go run ./cmd/focusd
```

## Troubleshooting

### Daemon is not running

If `focus` says the daemon is not running:

```bash
focus doctor
```

Then start the daemon:

```bash
go run ./cmd/focusd
```

### Socket path mismatch

If the client and daemon are using different socket paths, set the same value in both shells:

```bash
export FOCUS_SOCKET_PATH=/tmp/focus-dev.sock
```

### Invalid config

If the daemon exits during startup, the config may have a bad duration or an invalid timing relationship.

Check the config file and look for errors like:

- invalid `task.*`
- `break.warning` not before break start
- `break` window longer than the task duration

### Missing helper binary

If daemon events do not appear, verify `focus-events` is installed alongside the daemon in `~/.local/libexec/focus` (or your chosen prefix):

```bash
focus doctor
```

The daemon depends on the helper to emit `screen`, `sleep`, and `shutdown` events.

## Smoke-Test Workflow

For a fast manual test, use a dev config with small durations and disposable paths.

Typical setup:

```bash
FOCUS_CONFIG=./focus.dev.json \
FOCUS_SOCKET_PATH=/tmp/focus-dev.sock \
FOCUS_HISTORY_FILE=/tmp/focus-history.jsonl \
go run ./cmd/focusd
```

Then in another terminal:

```bash
FOCUS_SOCKET_PATH=/tmp/focus-dev.sock go run ./cmd/focus start --name demo --duration long
```

That lets you verify:

- break start
- break-end unlock
- cooldown start
- completion alert loop
- history persistence

## Notes

- `idle.warn_after` and `idle.lock_after` are daemon-side policy values.
- `focus-events` emits `screen`, `sleep`, and `shutdown` events over the helper pipe.
- The history log only stores completed tasks.
