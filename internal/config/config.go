package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"focus/internal/state"
)

type File struct {
	Cooldown cooldownJSON `json:"cooldown"`
	Break    breakJSON    `json:"break"`
	Idle     idleJSON     `json:"idle"`
	Alert    alertJSON    `json:"alert"`
}

type cooldownJSON struct {
	Short string `json:"short"`
	Long  string `json:"long"`
	Deep  string `json:"deep"`
}

type breakJSON struct {
	LongStart    string `json:"long_start"`
	DeepStart    string `json:"deep_start"`
	Warning      string `json:"warning"`
	LongDuration string `json:"long_duration"`
	DeepDuration string `json:"deep_duration"`
	RelockDelay  string `json:"relock_delay"`
}

type idleJSON struct {
	WarnAfter    string `json:"warn_after"`
	LockAfter    string `json:"lock_after"`
	PollInterval string `json:"poll_interval"`
}

type alertJSON struct {
	RepeatInterval string `json:"repeat_interval"`
}

type Overrides struct {
	CooldownShort *time.Duration
	CooldownLong  *time.Duration
	CooldownDeep  *time.Duration

	BreakLongStart    *time.Duration
	BreakDeepStart    *time.Duration
	BreakWarning      *time.Duration
	BreakLongDuration *time.Duration
	BreakDeepDuration *time.Duration
	BreakRelockDelay  *time.Duration

	IdleWarnAfter    *time.Duration
	IdleLockAfter    *time.Duration
	IdlePollInterval *time.Duration

	CompletionAlertRepeatInterval *time.Duration
}

func DefaultPath() (string, error) {
	if p := os.Getenv("FOCUS_CONFIG"); p != "" {
		return p, nil
	}
	return defaultPath()
}

func defaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}
	return filepath.Join(home, ".config", "focus", "config.json"), nil
}

func Load(path string) (File, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return File{}, false, nil
		}
		return File{}, false, fmt.Errorf("read config: %w", err)
	}

	var cfg File
	if err := json.Unmarshal(data, &cfg); err != nil {
		return File{}, true, fmt.Errorf("parse config JSON: %w", err)
	}
	return cfg, true, nil
}

func ResolveRuntimeConfig(defaults state.RuntimeConfig, fileCfg File, overrides Overrides) (state.RuntimeConfig, error) {
	resolved := defaults

	if err := applyDuration(&resolved.CooldownShort, fileCfg.Cooldown.Short); err != nil {
		return state.RuntimeConfig{}, fmt.Errorf("invalid cooldown.short: %w", err)
	}
	if err := applyDuration(&resolved.CooldownLong, fileCfg.Cooldown.Long); err != nil {
		return state.RuntimeConfig{}, fmt.Errorf("invalid cooldown.long: %w", err)
	}
	if err := applyDuration(&resolved.CooldownDeep, fileCfg.Cooldown.Deep); err != nil {
		return state.RuntimeConfig{}, fmt.Errorf("invalid cooldown.deep: %w", err)
	}

	if err := applyDuration(&resolved.BreakLongStart, fileCfg.Break.LongStart); err != nil {
		return state.RuntimeConfig{}, fmt.Errorf("invalid break.long_start: %w", err)
	}
	if err := applyDuration(&resolved.BreakDeepStart, fileCfg.Break.DeepStart); err != nil {
		return state.RuntimeConfig{}, fmt.Errorf("invalid break.deep_start: %w", err)
	}
	if err := applyDuration(&resolved.BreakWarning, fileCfg.Break.Warning); err != nil {
		return state.RuntimeConfig{}, fmt.Errorf("invalid break.warning: %w", err)
	}
	if err := applyDuration(&resolved.BreakLongDuration, fileCfg.Break.LongDuration); err != nil {
		return state.RuntimeConfig{}, fmt.Errorf("invalid break.long_duration: %w", err)
	}
	if err := applyDuration(&resolved.BreakDeepDuration, fileCfg.Break.DeepDuration); err != nil {
		return state.RuntimeConfig{}, fmt.Errorf("invalid break.deep_duration: %w", err)
	}
	if err := applyDuration(&resolved.BreakRelockDelay, fileCfg.Break.RelockDelay); err != nil {
		return state.RuntimeConfig{}, fmt.Errorf("invalid break.relock_delay: %w", err)
	}

	if err := applyDuration(&resolved.IdleWarnAfter, fileCfg.Idle.WarnAfter); err != nil {
		return state.RuntimeConfig{}, fmt.Errorf("invalid idle.warn_after: %w", err)
	}
	if err := applyDuration(&resolved.IdleLockAfter, fileCfg.Idle.LockAfter); err != nil {
		return state.RuntimeConfig{}, fmt.Errorf("invalid idle.lock_after: %w", err)
	}
	if err := applyDuration(&resolved.IdlePollInterval, fileCfg.Idle.PollInterval); err != nil {
		return state.RuntimeConfig{}, fmt.Errorf("invalid idle.poll_interval: %w", err)
	}
	if err := applyDuration(&resolved.CompletionAlertRepeatInterval, fileCfg.Alert.RepeatInterval); err != nil {
		return state.RuntimeConfig{}, fmt.Errorf("invalid alert.repeat_interval: %w", err)
	}

	applyOverride(&resolved.CooldownShort, overrides.CooldownShort)
	applyOverride(&resolved.CooldownLong, overrides.CooldownLong)
	applyOverride(&resolved.CooldownDeep, overrides.CooldownDeep)
	applyOverride(&resolved.BreakLongStart, overrides.BreakLongStart)
	applyOverride(&resolved.BreakDeepStart, overrides.BreakDeepStart)
	applyOverride(&resolved.BreakWarning, overrides.BreakWarning)
	applyOverride(&resolved.BreakLongDuration, overrides.BreakLongDuration)
	applyOverride(&resolved.BreakDeepDuration, overrides.BreakDeepDuration)
	applyOverride(&resolved.BreakRelockDelay, overrides.BreakRelockDelay)
	applyOverride(&resolved.IdleWarnAfter, overrides.IdleWarnAfter)
	applyOverride(&resolved.IdleLockAfter, overrides.IdleLockAfter)
	applyOverride(&resolved.IdlePollInterval, overrides.IdlePollInterval)
	applyOverride(&resolved.CompletionAlertRepeatInterval, overrides.CompletionAlertRepeatInterval)

	return resolved, nil
}

func ParseDuration(value string) (time.Duration, error) {
	d, err := time.ParseDuration(value)
	if err != nil {
		return 0, err
	}
	return d, nil
}

func applyDuration(dst *time.Duration, raw string) error {
	if raw == "" {
		return nil
	}
	value, err := ParseDuration(raw)
	if err != nil {
		return err
	}
	*dst = value
	return nil
}

func applyOverride(dst *time.Duration, value *time.Duration) {
	if value != nil {
		*dst = *value
	}
}
