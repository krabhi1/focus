package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"focus/internal/app"
	"focus/internal/effects"
	"focus/internal/events"
	"focus/internal/storage"
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

type durationFlag struct {
	value time.Duration
	set   bool
}

func (d *durationFlag) String() string {
	if d == nil || !d.set {
		return ""
	}
	return d.value.String()
}

func (d *durationFlag) Set(raw string) error {
	value, err := time.ParseDuration(raw)
	if err != nil {
		return err
	}
	d.value = value
	d.set = true
	return nil
}

func (d *durationFlag) Value() (*time.Duration, bool) {
	if d == nil || !d.set {
		return nil, false
	}
	value := d.value
	return &value, true
}

type runtimeFlagSet struct {
	taskShort  durationFlag
	taskMedium durationFlag
	taskLong   durationFlag
	taskDeep   durationFlag

	cooldownShort durationFlag
	cooldownLong  durationFlag
	cooldownDeep  durationFlag

	breakLongStart    durationFlag
	breakDeepStart    durationFlag
	breakWarning      durationFlag
	breakLongDuration durationFlag
	breakDeepDuration durationFlag
	relockDelay       durationFlag
	cooldownStart     durationFlag

	idleWarnAfter durationFlag
	idleLockAfter durationFlag

	eventsIdleThreshold durationFlag
	eventsIdlePoll      durationFlag

	alertRepeatInterval durationFlag
}

func main() {
	if err := run(); err != nil {
		log.Printf("fatal: %v", err)
		os.Exit(1)
	}
}

func run() error {
	flags := runtimeFlagSet{}
	configPath := flag.String("config", "", "Path to config JSON")
	flag.Var(&flags.taskShort, "task-short", "Override task.short")
	flag.Var(&flags.taskMedium, "task-medium", "Override task.medium")
	flag.Var(&flags.taskLong, "task-long", "Override task.long")
	flag.Var(&flags.taskDeep, "task-deep", "Override task.deep")
	flag.Var(&flags.cooldownShort, "cooldown-short", "Override cooldown.short")
	flag.Var(&flags.cooldownLong, "cooldown-long", "Override cooldown.long")
	flag.Var(&flags.cooldownDeep, "cooldown-deep", "Override cooldown.deep")
	flag.Var(&flags.breakLongStart, "break-long-start", "Override break.long_start")
	flag.Var(&flags.breakDeepStart, "break-deep-start", "Override break.deep_start")
	flag.Var(&flags.breakWarning, "break-warning", "Override break.warning")
	flag.Var(&flags.breakLongDuration, "break-long-duration", "Override break.long_duration")
	flag.Var(&flags.breakDeepDuration, "break-deep-duration", "Override break.deep_duration")
	flag.Var(&flags.relockDelay, "relock-delay", "Override relock_delay")
	flag.Var(&flags.cooldownStart, "cooldown-start-delay", "Override cooldown_start_delay")
	flag.Var(&flags.idleWarnAfter, "idle-warn-after", "Override idle.warn_after")
	flag.Var(&flags.idleLockAfter, "idle-lock-after", "Override idle.lock_after")
	flag.Var(&flags.eventsIdleThreshold, "events-idle-threshold", "Override helper idle threshold")
	flag.Var(&flags.eventsIdlePoll, "events-idle-poll", "Override helper idle poll")
	flag.Var(&flags.alertRepeatInterval, "completion-alert-repeat-interval", "Override alert.repeat_interval")
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := loadConfig(*configPath, flags.toOverrides()); err != nil {
		return err
	}
	warnMissingRuntimeDependencies(exec.LookPath)

	resolvedConfigPath, err := resolveConfigPath(*configPath)
	if err != nil {
		return err
	}
	historyPath, err := storage.DefaultHistoryPath()
	if err != nil {
		return err
	}
	socketPath := storage.DefaultSocketPath()
	if traceFlowEnabled() {
		log.Printf("paths config=%s socket=%s history=%s", resolvedConfigPath, socketPath, historyPath)
	}

	rt := app.NewRuntime(effects.RealActions{})
	defer rt.Close()
	if err := rt.LoadHistoryFromDisk(); err != nil {
		log.Printf("warning: failed to load history: %v", err)
	} else if traceFlowEnabled() {
		log.Printf("history loaded today_count=%d", rt.HistoryCount())
	}

	cfg := storage.GetRuntimeConfig()
	if traceFlowEnabled() {
		logRuntimeConfig(cfg)
	}
	listener, err := events.Start(ctx, int(cfg.EventsIdleThreshold/time.Second), int(cfg.EventsIdlePoll/time.Second))
	if err != nil {
		return fmt.Errorf("focus-events startup failed: %w", err)
	}
	go consumeHelperEvents(listener.Events, rt)
	helperFatal := make(chan error, 1)
	go monitorHelperErrors(listener.Errors, cancel, helperFatal)

	if err := ensureSocketPathAvailable(socketPath); err != nil {
		return err
	}
	l, err := net.Listen("unix", socketPath)
	if err != nil {
		return err
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

	srv := app.NewServer(rt, effects.RealActions{}, func() error { return loadConfig(*configPath, flags.toOverrides()) })
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

func (f runtimeFlagSet) toOverrides() storage.Overrides {
	overrides := storage.Overrides{}
	if v, ok := f.taskShort.Value(); ok {
		overrides.TaskShort = v
	}
	if v, ok := f.taskMedium.Value(); ok {
		overrides.TaskMedium = v
	}
	if v, ok := f.taskLong.Value(); ok {
		overrides.TaskLong = v
	}
	if v, ok := f.taskDeep.Value(); ok {
		overrides.TaskDeep = v
	}
	if v, ok := f.cooldownShort.Value(); ok {
		overrides.CooldownShort = v
	}
	if v, ok := f.cooldownLong.Value(); ok {
		overrides.CooldownLong = v
	}
	if v, ok := f.cooldownDeep.Value(); ok {
		overrides.CooldownDeep = v
	}
	if v, ok := f.breakLongStart.Value(); ok {
		overrides.BreakLongStart = v
	}
	if v, ok := f.breakDeepStart.Value(); ok {
		overrides.BreakDeepStart = v
	}
	if v, ok := f.breakWarning.Value(); ok {
		overrides.BreakWarning = v
	}
	if v, ok := f.breakLongDuration.Value(); ok {
		overrides.BreakLongDuration = v
	}
	if v, ok := f.breakDeepDuration.Value(); ok {
		overrides.BreakDeepDuration = v
	}
	if v, ok := f.relockDelay.Value(); ok {
		overrides.RelockDelay = v
	}
	if v, ok := f.cooldownStart.Value(); ok {
		overrides.CooldownStartDelay = v
	}
	if v, ok := f.idleWarnAfter.Value(); ok {
		overrides.IdleWarnAfter = v
	}
	if v, ok := f.idleLockAfter.Value(); ok {
		overrides.IdleLockAfter = v
	}
	if v, ok := f.eventsIdleThreshold.Value(); ok {
		overrides.EventsIdleThreshold = v
	}
	if v, ok := f.eventsIdlePoll.Value(); ok {
		overrides.EventsIdlePoll = v
	}
	if v, ok := f.alertRepeatInterval.Value(); ok {
		overrides.CompletionAlertRepeatInterval = v
	}
	return overrides
}

func loadConfig(path string, overrides storage.Overrides) error {
	if path == "" {
		p, err := storage.DefaultPath()
		if err != nil {
			return err
		}
		path = p
	}
	fileCfg, _, err := storage.Load(path)
	if err != nil {
		return err
	}
	cfg, err := storage.ResolveRuntimeConfig(storage.DefaultRuntimeConfig(), fileCfg, overrides)
	if err != nil {
		return err
	}
	return storage.SetRuntimeConfig(cfg)
}

func resolveConfigPath(path string) (string, error) {
	if path != "" {
		return path, nil
	}
	return storage.DefaultPath()
}

func warnMissingRuntimeDependencies(lookPath func(string) (string, error)) {
	deps := []struct {
		name     string
		required bool
	}{
		{name: "xdg-screensaver", required: true},
		{name: "notify-send", required: true},
		{name: "paplay", required: false},
	}
	for _, dep := range deps {
		if _, err := lookPath(dep.name); err == nil {
			continue
		}
		if dep.required {
			log.Printf("warning: missing dependency '%s' (required)", dep.name)
		} else {
			log.Printf("warning: missing dependency '%s' (optional)", dep.name)
		}
	}
}

func consumeHelperEvents(eventCh <-chan events.Event, runtime *app.Runtime) {
	for event := range eventCh {
		if traceFlowEnabled() {
			log.Printf("focus-events event=%s state=%s fields=%v", event.Kind, event.State, event.Fields)
		}
		switch event.Kind {
		case events.KindIdle:
			switch event.State {
			case "entered":
				runtime.OnIdleEntered()
			case "exited":
				runtime.OnIdleExited()
			}
		case events.KindScreen:
			switch event.State {
			case "locked":
				runtime.SetSystemLocked(true)
				runtime.OnScreenLocked()
			case "unlocked":
				runtime.SetSystemLocked(false)
				runtime.OnScreenUnlocked()
			}
		}
	}
}

func traceFlowEnabled() bool {
	return os.Getenv("FOCUS_TRACE_FLOW") == "1"
}

func logRuntimeConfig(cfg storage.RuntimeConfig) {
	log.Printf(
		"runtime config task=[%s,%s,%s,%s] cooldown=[%s,%s,%s start:%s] break=[start:%s/%s dur:%s/%s warn:%s relock:%s] no-task=[warn:%s lock:%s] events=[threshold:%s poll:%s] alert=[repeat:%s]",
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

func isHelperFatalError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "focus-events exited")
}

func monitorHelperErrors(errCh <-chan error, cancel context.CancelFunc, helperFatal chan<- error) {
	for err := range errCh {
		if err == nil {
			continue
		}
		log.Printf("focus-events error: %v", err)
		if isHelperFatalError(err) {
			select {
			case helperFatal <- err:
			default:
			}
			cancel()
			return
		}
	}
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
		return err
	}
	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("refusing to remove non-socket path: %s", path)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove stale socket: %w", err)
	}
	return nil
}
