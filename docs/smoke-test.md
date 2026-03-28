# Manual Smoke Test

Use this to verify the full task flow quickly with short durations.

## Dev Config

Create `focus.dev.json` in the repo root:

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
    "repeat_interval": "1s"
  }
}
```

## Run Daemon

Start the daemon with disposable paths:

```bash
FOCUS_CONFIG=./focus.dev.json \
FOCUS_SOCKET_PATH=/tmp/focus-dev.sock \
FOCUS_HISTORY_FILE=/tmp/focus-history.jsonl \
FOCUS_TRACE_FLOW=1 \
go run ./cmd/focusd --events-idle-threshold 5s --events-idle-poll 1s
```

## Exercise The Flow

In another terminal:

```bash
FOCUS_SOCKET_PATH=/tmp/focus-dev.sock go run ./cmd/focus start --name demo --duration long
```

Expected flow:

1. Break starts after about 5 seconds.
2. The screen locks when break starts.
3. If you unlock during break or cooldown, it re-locks after the relock delay.
4. Break ends after 3 seconds and unlocks the screen.
5. The completion sound repeats every second while the user is idle.
6. The sound stops when `focus-events` reports user activity again or when the screen unlocks.
7. Cooldown starts after task completion and blocks new tasks until it expires.

## Verify State

Check status and history:

```bash
FOCUS_SOCKET_PATH=/tmp/focus-dev.sock go run ./cmd/focus status
FOCUS_SOCKET_PATH=/tmp/focus-dev.sock go run ./cmd/focus history
```

## Cleanup

```bash
rm -f /tmp/focus-dev.sock /tmp/focus-history.jsonl ./focus.dev.json
```
