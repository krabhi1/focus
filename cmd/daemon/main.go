package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"focus/internal/config"
	"focus/internal/events"
	"focus/internal/state"
	"focus/internal/sys"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

const (
	idleThresholdSeconds = 300
	idlePollSeconds      = 1
)

func main() {
	opts := parseDaemonOptions()

	sigCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	ctx, cancel := context.WithCancel(sigCtx)
	defer cancel()

	socketPath := ""
	applyConfig := func() error {
		return loadDaemonConfig(opts)
	}

	if err := applyConfig(); err != nil {
		log.Printf("config startup failed: %v", err)
		return
	}
	warnMissingRuntimeDependencies(exec.LookPath)

	srv := NewServer(state.Get(), sys.RealActions{}, applyConfig)

	listener, err := events.Start(ctx, idleThresholdSeconds, idlePollSeconds)
	if err != nil {
		log.Printf("focus-events startup failed: %v", err)
	} else {
		go consumeHelperEvents(listener.Events)
		go logHelperErrors(listener.Errors)
	}

	socketPath = state.DefaultSocketPath()

	if err := ensureSocketPathAvailable(socketPath); err != nil {
		log.Printf("socket path setup failed: %v", err)
		return
	}

	l, err := net.Listen("unix", socketPath)
	if err != nil {
		fmt.Printf("Listen error: %v\n", err)
		return
	}
	defer func() {
		_ = l.Close()
		_ = os.Remove(socketPath)
	}()

	fmt.Println("Go Daemon listening on", socketPath)
	go state.Get().StartIdleMonitor(ctx)

	go func() {
		<-ctx.Done()
		_ = l.Close()
	}()

	for {
		conn, err := l.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			continue
		}

		go srv.HandleConnection(conn)
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
	opts.overrides.CooldownShort = fs.Duration("cooldown-short", 0, "Override cooldown.short duration")
	opts.overrides.CooldownLong = fs.Duration("cooldown-long", 0, "Override cooldown.long duration")
	opts.overrides.CooldownDeep = fs.Duration("cooldown-deep", 0, "Override cooldown.deep duration")
	opts.overrides.BreakLongStart = fs.Duration("break-long-start", 0, "Override break.long_start duration")
	opts.overrides.BreakDeepStart = fs.Duration("break-deep-start", 0, "Override break.deep_start duration")
	opts.overrides.BreakWarning = fs.Duration("break-warning", 0, "Override break.warning duration")
	opts.overrides.BreakLongDuration = fs.Duration("break-long-duration", 0, "Override break.long_duration duration")
	opts.overrides.BreakDeepDuration = fs.Duration("break-deep-duration", 0, "Override break.deep_duration duration")
	opts.overrides.BreakRelockDelay = fs.Duration("break-relock-delay", 0, "Override break.relock_delay duration")
	opts.overrides.IdleWarnAfter = fs.Duration("idle-warn-after", 0, "Override idle.warn_after duration")
	opts.overrides.IdleLockAfter = fs.Duration("idle-lock-after", 0, "Override idle.lock_after duration")
	opts.overrides.IdlePollInterval = fs.Duration("idle-poll-interval", 0, "Override idle.poll_interval duration")
	_ = fs.Parse(os.Args[1:])
	normalizeDurationOverrides(&opts.overrides)
	return opts
}

func loadDaemonConfig(opts daemonOptions) error {
	configPath := opts.configPath
	if configPath == "" {
		defaultPath, err := config.DefaultPath()
		if err != nil {
			return err
		}
		configPath = defaultPath
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

func normalizeDurationOverrides(overrides *config.Overrides) {
	normalize := func(value *time.Duration) *time.Duration {
		if value == nil || *value == 0 {
			return nil
		}
		return value
	}

	overrides.CooldownShort = normalize(overrides.CooldownShort)
	overrides.CooldownLong = normalize(overrides.CooldownLong)
	overrides.CooldownDeep = normalize(overrides.CooldownDeep)
	overrides.BreakLongStart = normalize(overrides.BreakLongStart)
	overrides.BreakDeepStart = normalize(overrides.BreakDeepStart)
	overrides.BreakWarning = normalize(overrides.BreakWarning)
	overrides.BreakLongDuration = normalize(overrides.BreakLongDuration)
	overrides.BreakDeepDuration = normalize(overrides.BreakDeepDuration)
	overrides.BreakRelockDelay = normalize(overrides.BreakRelockDelay)
	overrides.IdleWarnAfter = normalize(overrides.IdleWarnAfter)
	overrides.IdleLockAfter = normalize(overrides.IdleLockAfter)
	overrides.IdlePollInterval = normalize(overrides.IdlePollInterval)
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

func consumeHelperEvents(eventCh <-chan events.Event) {
	for event := range eventCh {
		switch event.Kind {
		case events.KindScreen:
			switch event.State {
			case "locked":
				state.Get().SetSystemLocked(true)
				state.Get().OnScreenLocked()
			case "unlocked":
				state.Get().SetSystemLocked(false)
				state.Get().OnScreenUnlocked()
			}
		}
		log.Printf("focus-events event=%s state=%s fields=%v", event.Kind, event.State, event.Fields)
	}
}

func logHelperErrors(errCh <-chan error) {
	for err := range errCh {
		if err != nil {
			log.Printf("focus-events error: %v", err)
		}
	}
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
