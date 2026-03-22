package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"focus/internal/config"
	"focus/internal/core"
	"focus/internal/events"
	"focus/internal/state"
	"focus/internal/sys"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func main() {
	if err := run(); err != nil {
		log.Printf("fatal: %v", err)
		os.Exit(1)
	}
}

func run() error {
	opts := parseDaemonOptions()

	sigCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	ctx, cancel := context.WithCancel(sigCtx)
	defer cancel()

	applyConfig := func() error {
		return loadDaemonConfig(opts)
	}

	if err := applyConfig(); err != nil {
		return fmt.Errorf("config startup failed: %w", err)
	}
	warnMissingRuntimeDependencies(exec.LookPath)

	rt := NewDaemonRuntime(sys.RealActions{})
	defer rt.Close()
	srv := NewServer(rt, sys.RealActions{}, applyConfig)
	srv.SetStatusProvider(func() string {
		return formatCoreStatus(rt.CoreSnapshot())
	})

	configPath, err := resolvedConfigPath(opts)
	if err != nil {
		return err
	}
	socketPath := state.DefaultSocketPath()
	historyPath, err := state.DefaultHistoryPath()
	if err != nil {
		return err
	}
	if traceFlowEnabled() {
		log.Printf("paths config=%s socket=%s history=%s", configPath, socketPath, historyPath)
	}

	if err := rt.LoadHistoryFromDisk(); err != nil {
		log.Printf("warning: failed to load persisted history: %v", err)
	} else if traceFlowEnabled() {
		log.Printf("history loaded today_count=%d", rt.HistoryCount())
	}

	runtimeCfg := state.GetRuntimeConfig()
	if traceFlowEnabled() {
		logRuntimeConfig(runtimeCfg)
	}
	listener, err := events.Start(ctx, int(runtimeCfg.EventsIdleThreshold/time.Second), int(runtimeCfg.EventsIdlePoll/time.Second))
	if err != nil {
		return fmt.Errorf("focus-events startup failed: %w", err)
	}
	go consumeHelperEvents(listener.Events, rt)
	helperFatal := make(chan error, 1)
	go monitorHelperErrors(listener.Errors, cancel, helperFatal)

	if err := ensureSocketPathAvailable(socketPath); err != nil {
		return fmt.Errorf("socket path setup failed: %w", err)
	}

	l, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("listen error: %w", err)
	}
	defer func() {
		_ = l.Close()
		_ = os.Remove(socketPath)
	}()

	fmt.Println("Go Daemon listening on", socketPath)

	go func() {
		<-ctx.Done()
		_ = l.Close()
	}()

	for {
		conn, err := l.Accept()
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			continue
		}

		go srv.HandleConnection(conn)
	}

	select {
	case err := <-helperFatal:
		return err
	default:
		return nil
	}
}

type daemonOptions struct {
	configPath string

	overrides config.Overrides
}

func parseDaemonOptions() daemonOptions {
	var opts daemonOptions
	fs := flag.NewFlagSet("focusd", flag.ExitOnError)
	fs.StringVar(&opts.configPath, "config", "", "Path to config JSON (default: $FOCUS_CONFIG or ~/.config/focus/config.json)")
	opts.overrides.TaskShort = fs.Duration("task-short", 0, "Override task.short duration")
	opts.overrides.TaskMedium = fs.Duration("task-medium", 0, "Override task.medium duration")
	opts.overrides.TaskLong = fs.Duration("task-long", 0, "Override task.long duration")
	opts.overrides.TaskDeep = fs.Duration("task-deep", 0, "Override task.deep duration")
	opts.overrides.CooldownShort = fs.Duration("cooldown-short", 0, "Override cooldown.short duration")
	opts.overrides.CooldownLong = fs.Duration("cooldown-long", 0, "Override cooldown.long duration")
	opts.overrides.CooldownDeep = fs.Duration("cooldown-deep", 0, "Override cooldown.deep duration")
	opts.overrides.BreakLongStart = fs.Duration("break-long-start", 0, "Override break.long_start duration")
	opts.overrides.BreakDeepStart = fs.Duration("break-deep-start", 0, "Override break.deep_start duration")
	opts.overrides.BreakWarning = fs.Duration("break-warning", 0, "Override break.warning duration")
	opts.overrides.BreakLongDuration = fs.Duration("break-long-duration", 0, "Override break.long_duration duration")
	opts.overrides.BreakDeepDuration = fs.Duration("break-deep-duration", 0, "Override break.deep_duration duration")
	opts.overrides.RelockDelay = fs.Duration("relock-delay", 0, "Override relock_delay duration")
	opts.overrides.CooldownStartDelay = fs.Duration("cooldown-start-delay", 0, "Override cooldown_start_delay duration")
	opts.overrides.IdleWarnAfter = fs.Duration("idle-warn-after", 0, "Override idle.warn_after duration")
	opts.overrides.IdleLockAfter = fs.Duration("idle-lock-after", 0, "Override idle.lock_after duration")
	opts.overrides.EventsIdleThreshold = fs.Duration("events-idle-threshold", 0, "Override events.idle_threshold duration")
	opts.overrides.EventsIdlePoll = fs.Duration("events-idle-poll", 0, "Override events.idle_poll duration")
	opts.overrides.CompletionAlertRepeatInterval = fs.Duration("completion-alert-repeat-interval", 0, "Override alert.repeat_interval duration")
	_ = fs.Parse(os.Args[1:])
	normalizeDurationOverrides(&opts.overrides)
	return opts
}

func loadDaemonConfig(opts daemonOptions) error {
	configPath, err := resolvedConfigPath(opts)
	if err != nil {
		return err
	}

	fileCfg, _, err := config.Load(configPath)
	if err != nil {
		return err
	}

	runtimeCfg, err := config.ResolveRuntimeConfig(state.DefaultRuntimeConfig(), fileCfg, opts.overrides)
	if err != nil {
		return err
	}
	if err := state.SetRuntimeConfig(runtimeCfg); err != nil {
		return err
	}

	return nil
}

func resolvedConfigPath(opts daemonOptions) (string, error) {
	if opts.configPath != "" {
		return opts.configPath, nil
	}
	return config.DefaultPath()
}

func normalizeDurationOverrides(overrides *config.Overrides) {
	normalize := func(value *time.Duration) *time.Duration {
		if value == nil || *value == 0 {
			return nil
		}
		return value
	}

	overrides.CooldownShort = normalize(overrides.CooldownShort)
	overrides.TaskShort = normalize(overrides.TaskShort)
	overrides.TaskMedium = normalize(overrides.TaskMedium)
	overrides.TaskLong = normalize(overrides.TaskLong)
	overrides.TaskDeep = normalize(overrides.TaskDeep)
	overrides.CooldownLong = normalize(overrides.CooldownLong)
	overrides.CooldownDeep = normalize(overrides.CooldownDeep)
	overrides.BreakLongStart = normalize(overrides.BreakLongStart)
	overrides.BreakDeepStart = normalize(overrides.BreakDeepStart)
	overrides.BreakWarning = normalize(overrides.BreakWarning)
	overrides.BreakLongDuration = normalize(overrides.BreakLongDuration)
	overrides.BreakDeepDuration = normalize(overrides.BreakDeepDuration)
	overrides.RelockDelay = normalize(overrides.RelockDelay)
	overrides.CooldownStartDelay = normalize(overrides.CooldownStartDelay)
	overrides.IdleWarnAfter = normalize(overrides.IdleWarnAfter)
	overrides.IdleLockAfter = normalize(overrides.IdleLockAfter)
	overrides.EventsIdleThreshold = normalize(overrides.EventsIdleThreshold)
	overrides.EventsIdlePoll = normalize(overrides.EventsIdlePoll)
	overrides.CompletionAlertRepeatInterval = normalize(overrides.CompletionAlertRepeatInterval)
}

func ensureSocketPathAvailable(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create socket directory: %w", err)
	}

	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("stat socket path: %w", err)
	}

	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("refusing to remove non-socket path: %s", path)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove stale socket: %w", err)
	}
	return nil
}

func consumeHelperEvents(eventCh <-chan events.Event, runtime *DaemonRuntime) {
	for event := range eventCh {
		now := runtime.Now()
		if traceFlowEnabled() {
			log.Printf("focus-events event=%s state=%s fields=%v", event.Kind, event.State, event.Fields)
		}
		switch event.Kind {
		case events.KindIdle:
			switch event.State {
			case "entered":
				runtime.OnIdleEntered()
				runtime.PublishCoreEvent(core.Event{Type: core.EventIdleEntered, At: now})
			case "exited":
				runtime.OnIdleExited()
				runtime.PublishCoreEvent(core.Event{Type: core.EventIdleExited, At: now})
			}
		case events.KindScreen:
			switch event.State {
			case "locked":
				runtime.SetSystemLocked(true)
				runtime.OnScreenLocked()
				runtime.PublishCoreEvent(core.Event{Type: core.EventScreenLocked, At: now})
			case "unlocked":
				runtime.SetSystemLocked(false)
				runtime.OnScreenUnlocked()
				runtime.PublishCoreEvent(core.Event{Type: core.EventScreenUnlock, At: now})
			}
		}
	}
}

func logRuntimeConfig(cfg state.RuntimeConfig) {
	log.Printf(
		"runtime config task=[%s,%s,%s,%s] cooldown=[%s,%s,%s start:%s] break=[start:%s/%s dur:%s/%s warn:%s relock:%s] idle=[warn:%s lock:%s] events=[threshold:%s poll:%s] alert=[repeat:%s]",
		cfg.TaskShort,
		cfg.TaskMedium,
		cfg.TaskLong,
		cfg.TaskDeep,
		cfg.CooldownShort,
		cfg.CooldownLong,
		cfg.CooldownDeep,
		cfg.CooldownStartDelay,
		cfg.BreakLongStart,
		cfg.BreakDeepStart,
		cfg.BreakLongDuration,
		cfg.BreakDeepDuration,
		cfg.BreakWarning,
		cfg.RelockDelay,
		cfg.IdleWarnAfter,
		cfg.IdleLockAfter,
		cfg.EventsIdleThreshold,
		cfg.EventsIdlePoll,
		cfg.CompletionAlertRepeatInterval,
	)
}

func formatCoreStatus(snapshot core.State) string {
	now := time.Now()
	switch snapshot.Phase {
	case core.PhasePendingCooldown:
		remaining := snapshot.CooldownStartUntil.Sub(now)
		if remaining < 0 {
			remaining = 0
		}
		return fmt.Sprintf("Cooldown starting | Remaining: %s", remaining.Round(time.Second))
	case core.PhaseCooldown:
		remaining := snapshot.CooldownUntil.Sub(now)
		if remaining < 0 {
			remaining = 0
		}
		return fmt.Sprintf("Cooldown active | Remaining: %s", remaining.Round(time.Second))
	case core.PhaseBreak:
		remaining := snapshot.BreakUntil.Sub(now)
		if remaining < 0 {
			remaining = 0
		}
		return fmt.Sprintf("Status: break | Break remaining: %s", remaining.Round(time.Second))
	case core.PhaseActive:
		return "Task active"
	default:
		return "Idle"
	}
}

func monitorHelperErrors(errCh <-chan error, cancel context.CancelFunc, helperFatal chan<- error) {
	for err := range errCh {
		if err == nil {
			continue
		}
		log.Printf("focus-events error: %v", err)
		if isHelperFatalError(err) {
			cancel()
			select {
			case helperFatal <- err:
			default:
			}
			return
		}
	}
}

func isHelperFatalError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "focus-events exited")
}

func traceFlowEnabled() bool {
	return os.Getenv("FOCUS_TRACE_FLOW") == "1"
}

func warnMissingRuntimeDependencies(lookPath func(string) (string, error)) {
	if lookPath == nil {
		return
	}

	deps := []struct {
		command string
		impact  string
	}{
		{command: "xdg-screensaver", impact: "screen lock action will fail"},
		{command: "notify-send", impact: "desktop notifications will fail"},
		{command: "paplay", impact: "task-ending sound will fail"},
	}

	for _, dep := range deps {
		if _, err := lookPath(dep.command); err != nil {
			log.Printf("warning: missing dependency '%s': %s", dep.command, dep.impact)
		}
	}
}
