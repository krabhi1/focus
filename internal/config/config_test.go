package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"focus/internal/state"
)

func TestLoadMissingFileIsNonFatal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.json")
	_, exists, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if exists {
		t.Fatal("exists = true, want false")
	}
}

func TestResolveRuntimeConfigAppliesFileAndOverrides(t *testing.T) {
	defaults := state.DefaultRuntimeConfig()
	fileCfg := File{
		Task:     taskJSON{Short: "10m", Medium: "20m", Long: "40m", Deep: "70m"},
		Cooldown: cooldownJSON{Long: "12m"},
		Break:    breakJSON{Warning: "90s"},
		Idle:     idleJSON{WarnAfter: "4m"},
		Alert:    alertJSON{RepeatInterval: "7s"},
	}
	override := 8 * time.Minute
	cfg, err := ResolveRuntimeConfig(defaults, fileCfg, Overrides{
		IdleLockAfter: &override,
	})
	if err != nil {
		t.Fatalf("ResolveRuntimeConfig returned error: %v", err)
	}

	if cfg.CooldownLong != 12*time.Minute {
		t.Fatalf("CooldownLong = %s, want 12m", cfg.CooldownLong)
	}
	if cfg.TaskShort != 10*time.Minute || cfg.TaskMedium != 20*time.Minute || cfg.TaskLong != 40*time.Minute || cfg.TaskDeep != 70*time.Minute {
		t.Fatalf("task preset durations not resolved from config: %+v", cfg)
	}
	if cfg.BreakWarning != 90*time.Second {
		t.Fatalf("BreakWarning = %s, want 90s", cfg.BreakWarning)
	}
	if cfg.IdleWarnAfter != 4*time.Minute {
		t.Fatalf("IdleWarnAfter = %s, want 4m", cfg.IdleWarnAfter)
	}
	if cfg.IdleLockAfter != 8*time.Minute {
		t.Fatalf("IdleLockAfter = %s, want 8m override", cfg.IdleLockAfter)
	}
	if cfg.CompletionAlertRepeatInterval != 7*time.Second {
		t.Fatalf("CompletionAlertRepeatInterval = %s, want 7s", cfg.CompletionAlertRepeatInterval)
	}
	if cfg.EventsIdleThreshold != defaults.EventsIdleThreshold {
		t.Fatalf("EventsIdleThreshold = %s, want default %s", cfg.EventsIdleThreshold, defaults.EventsIdleThreshold)
	}
	if cfg.EventsIdlePoll != defaults.EventsIdlePoll {
		t.Fatalf("EventsIdlePoll = %s, want default %s", cfg.EventsIdlePoll, defaults.EventsIdlePoll)
	}
}

func TestResolveRuntimeConfigRejectsBreakWindowOverflow(t *testing.T) {
	defaults := state.DefaultRuntimeConfig()
	cfg, err := ResolveRuntimeConfig(defaults, File{
		Task:  taskJSON{Short: "5m", Medium: "10m", Long: "20m", Deep: "40m"},
		Break: breakJSON{Warning: "2m", LongStart: "15m", LongDuration: "10m", DeepStart: "30m", DeepDuration: "5m"},
	}, Overrides{})
	if err != nil {
		t.Fatalf("ResolveRuntimeConfig returned parse error: %v", err)
	}
	err = state.SetRuntimeConfig(cfg)
	if err == nil {
		t.Fatal("expected invalid break window error")
	}
	if !strings.Contains(err.Error(), "break.long_start + break.long_duration") {
		t.Fatalf("error = %q, want break window context", err.Error())
	}
}

func TestResolveRuntimeConfigRejectsInvalidDuration(t *testing.T) {
	defaults := state.DefaultRuntimeConfig()
	_, err := ResolveRuntimeConfig(defaults, File{
		Idle: idleJSON{WarnAfter: "bad"},
	}, Overrides{})
	if err == nil {
		t.Fatal("expected invalid duration error")
	}
	if !strings.Contains(err.Error(), "idle.warn_after") {
		t.Fatalf("error = %q, want idle.warn_after context", err.Error())
	}
}

func TestDefaultPathUsesFocusConfigEnv(t *testing.T) {
	t.Setenv("FOCUS_CONFIG", "/tmp/focus-custom-config.json")
	got, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath returned error: %v", err)
	}
	if got != "/tmp/focus-custom-config.json" {
		t.Fatalf("DefaultPath() = %q, want env override", got)
	}
}

func TestLoadParsesJSONFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	body := `{"idle":{"warn_after":"10s","lock_after":"20s"}}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	cfg, exists, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if !exists {
		t.Fatal("exists = false, want true")
	}
	if cfg.Idle.WarnAfter != "10s" {
		t.Fatalf("idle.warn_after = %q, want 10s", cfg.Idle.WarnAfter)
	}
}
