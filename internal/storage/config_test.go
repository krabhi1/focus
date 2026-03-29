package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	defaults := DefaultRuntimeConfig()
	fileCfg := File{
		Task:               taskJSON{Short: "10m", Medium: "20m", Long: "40m", Deep: "70m"},
		Cooldown:           cooldownJSON{Long: "12m"},
		Break:              breakJSON{Warning: "90s"},
		Idle:               idleJSON{WarnAfter: "4m"},
		Alert:              alertJSON{RepeatCount: ptrInt(7)},
		RelockDelay:        "45s",
		CooldownStartDelay: "11s",
	}
	override := 8 * time.Minute
	relockOverride := 9 * time.Minute
	cfg, err := ResolveRuntimeConfig(defaults, fileCfg, Overrides{
		IdleLockAfter: &override,
		RelockDelay:   &relockOverride,
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
	if cfg.RelockDelay != 9*time.Minute {
		t.Fatalf("RelockDelay = %s, want 9m override", cfg.RelockDelay)
	}
	if cfg.CooldownStartDelay != 11*time.Second {
		t.Fatalf("CooldownStartDelay = %s, want 11s", cfg.CooldownStartDelay)
	}
	if cfg.IdleWarnAfter != 4*time.Minute {
		t.Fatalf("IdleWarnAfter = %s, want 4m", cfg.IdleWarnAfter)
	}
	if cfg.IdleLockAfter != 8*time.Minute {
		t.Fatalf("IdleLockAfter = %s, want 8m override", cfg.IdleLockAfter)
	}
	if cfg.CompletionAlertRepeatCount != 7 {
		t.Fatalf("CompletionAlertRepeatCount = %d, want 7", cfg.CompletionAlertRepeatCount)
	}
}

func TestResolveRuntimeConfigRejectsBreakWindowOverflow(t *testing.T) {
	defaults := DefaultRuntimeConfig()
	cfg, err := ResolveRuntimeConfig(defaults, File{
		Task:        taskJSON{Short: "5m", Medium: "10m", Long: "20m", Deep: "40m"},
		Break:       breakJSON{Warning: "2m", LongStart: "15m", LongDuration: "10m", DeepStart: "30m", DeepDuration: "5m"},
		RelockDelay: "5s",
	}, Overrides{})
	if err != nil {
		t.Fatalf("ResolveRuntimeConfig returned parse error: %v", err)
	}
	err = SetRuntimeConfig(cfg)
	if err == nil {
		t.Fatal("expected invalid break window error")
	}
	if !strings.Contains(err.Error(), "break.long_start + break.long_duration") {
		t.Fatalf("error = %q, want break window context", err.Error())
	}
}

func TestResolveRuntimeConfigRejectsInvalidDuration(t *testing.T) {
	defaults := DefaultRuntimeConfig()
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

func TestResolveRuntimeConfigAllowsZeroAlertRepeatCount(t *testing.T) {
	defaults := DefaultRuntimeConfig()
	cfg, err := ResolveRuntimeConfig(defaults, File{
		Alert: alertJSON{RepeatCount: ptrInt(0)},
	}, Overrides{})
	if err != nil {
		t.Fatalf("ResolveRuntimeConfig returned error: %v", err)
	}
	if cfg.CompletionAlertRepeatCount != 0 {
		t.Fatalf("CompletionAlertRepeatCount = %d, want 0", cfg.CompletionAlertRepeatCount)
	}
}

func TestResolveRuntimeConfigAllowsZeroRelockDelay(t *testing.T) {
	defaults := DefaultRuntimeConfig()
	cfg, err := ResolveRuntimeConfig(defaults, File{
		RelockDelay: "0s",
	}, Overrides{})
	if err != nil {
		t.Fatalf("ResolveRuntimeConfig returned error: %v", err)
	}
	if cfg.RelockDelay != 0 {
		t.Fatalf("RelockDelay = %s, want 0s", cfg.RelockDelay)
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
	body := `{"idle":{"warn_after":"10s","lock_after":"20s"},"relock_delay":"30s","cooldown_start_delay":"10s","alert":{"repeat_count":4}}`
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
	if cfg.RelockDelay != "30s" {
		t.Fatalf("relock_delay = %q, want 30s", cfg.RelockDelay)
	}
	if cfg.CooldownStartDelay != "10s" {
		t.Fatalf("cooldown_start_delay = %q, want 10s", cfg.CooldownStartDelay)
	}
	if cfg.Alert.RepeatCount == nil || *cfg.Alert.RepeatCount != 4 {
		t.Fatalf("alert.repeat_count = %#v, want 4", cfg.Alert.RepeatCount)
	}
}

func TestLoadRejectsLegacyAlertRepeatInterval(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	body := `{"alert":{"repeat_interval":"3s"}}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	_, _, err := Load(path)
	if err == nil {
		t.Fatal("expected error for legacy alert.repeat_interval")
	}
	if !strings.Contains(err.Error(), "alert.repeat_count") {
		t.Fatalf("error = %q, want alert.repeat_count guidance", err.Error())
	}
}

func TestUpdateConfigValueWritesAndPreservesOtherFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	body := `{"idle":{"warn_after":"1m","lock_after":"2m"},"relock_delay":"0s"}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	if err := UpdateConfigValue(path, "idle.lock_after", "3m"); err != nil {
		t.Fatalf("UpdateConfigValue returned error: %v", err)
	}

	cfg, exists, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if !exists {
		t.Fatal("exists = false, want true")
	}
	if cfg.Idle.WarnAfter != "1m" {
		t.Fatalf("idle.warn_after = %q, want 1m", cfg.Idle.WarnAfter)
	}
	if cfg.Idle.LockAfter != "3m" {
		t.Fatalf("idle.lock_after = %q, want 3m", cfg.Idle.LockAfter)
	}
	if cfg.RelockDelay != "0s" {
		t.Fatalf("relock_delay = %q, want 0s", cfg.RelockDelay)
	}
}

func TestUpdateConfigValueRejectsUnknownKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	err := UpdateConfigValue(path, "bad.key", "1m")
	if err == nil {
		t.Fatal("expected error for unknown key")
	}
	if !strings.Contains(err.Error(), "unknown config key") {
		t.Fatalf("error = %q, want unknown config key", err.Error())
	}
}

func TestUpdateConfigValueRejectsInvalidDuration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	err := UpdateConfigValue(path, "idle.warn_after", "bad")
	if err == nil {
		t.Fatal("expected invalid duration error")
	}
	if !strings.Contains(err.Error(), "invalid duration") && !strings.Contains(err.Error(), "parse duration") {
		t.Fatalf("error = %q, want duration parse failure", err.Error())
	}
}

func TestUpdateConfigValueRejectsInvalidResultingConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"idle":{"warn_after":"1m","lock_after":"2m"}}`), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	err := UpdateConfigValue(path, "idle.warn_after", "3m")
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "idle.warn_after must be less than idle.lock_after") {
		t.Fatalf("error = %q, want idle warning validation failure", err.Error())
	}
}

func ptrInt(v int) *int {
	return &v
}
