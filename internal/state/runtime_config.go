package state

import (
	"fmt"
	"sync"
	"time"
)

type RuntimeConfig struct {
	TaskShort  time.Duration
	TaskMedium time.Duration
	TaskLong   time.Duration
	TaskDeep   time.Duration

	CooldownShort time.Duration
	CooldownLong  time.Duration
	CooldownDeep  time.Duration

	BreakLongStart    time.Duration
	BreakDeepStart    time.Duration
	BreakWarning      time.Duration
	BreakLongDuration time.Duration
	BreakDeepDuration time.Duration
	BreakRelockDelay  time.Duration

	IdleWarnAfter                 time.Duration
	IdleLockAfter                 time.Duration
	IdlePollInterval              time.Duration
	CompletionAlertRepeatInterval time.Duration
}

var (
	runtimeConfigMu sync.RWMutex
	runtimeConfig   = DefaultRuntimeConfig()
)

func DefaultRuntimeConfig() RuntimeConfig {
	return RuntimeConfig{
		TaskShort:  15 * time.Minute,
		TaskMedium: 30 * time.Minute,
		TaskLong:   60 * time.Minute,
		TaskDeep:   90 * time.Minute,

		CooldownShort: ShortCooldownDuration,
		CooldownLong:  LongCooldownDuration,
		CooldownDeep:  DeepCooldownDuration,

		BreakLongStart:    LongTaskBreakStartOffset,
		BreakDeepStart:    DeepTaskBreakStartOffset,
		BreakWarning:      BreakWarningOffset,
		BreakLongDuration: 5 * time.Minute,
		BreakDeepDuration: 10 * time.Minute,
		BreakRelockDelay:  BreakRelockDelay,

		IdleWarnAfter:                 IdleWarningAfter,
		IdleLockAfter:                 IdleLockAfter,
		IdlePollInterval:              IdleMonitorInterval,
		CompletionAlertRepeatInterval: 3 * time.Second,
	}
}

func SetRuntimeConfig(cfg RuntimeConfig) error {
	if err := validateRuntimeConfig(cfg); err != nil {
		return err
	}

	runtimeConfigMu.Lock()
	defer runtimeConfigMu.Unlock()
	runtimeConfig = cfg
	return nil
}

func GetRuntimeConfig() RuntimeConfig {
	runtimeConfigMu.RLock()
	defer runtimeConfigMu.RUnlock()
	return runtimeConfig
}

func validateRuntimeConfig(cfg RuntimeConfig) error {
	positive := []struct {
		name  string
		value time.Duration
	}{
		{"task.short", cfg.TaskShort},
		{"task.medium", cfg.TaskMedium},
		{"task.long", cfg.TaskLong},
		{"task.deep", cfg.TaskDeep},
		{"cooldown.short", cfg.CooldownShort},
		{"cooldown.long", cfg.CooldownLong},
		{"cooldown.deep", cfg.CooldownDeep},
		{"break.long_start", cfg.BreakLongStart},
		{"break.deep_start", cfg.BreakDeepStart},
		{"break.warning", cfg.BreakWarning},
		{"break.long_duration", cfg.BreakLongDuration},
		{"break.deep_duration", cfg.BreakDeepDuration},
		{"break.relock_delay", cfg.BreakRelockDelay},
		{"idle.warn_after", cfg.IdleWarnAfter},
		{"idle.lock_after", cfg.IdleLockAfter},
		{"idle.poll_interval", cfg.IdlePollInterval},
		{"alert.repeat_interval", cfg.CompletionAlertRepeatInterval},
	}

	for _, item := range positive {
		if item.value <= 0 {
			return fmt.Errorf("%s must be > 0", item.name)
		}
	}

	if !(cfg.TaskShort < cfg.TaskMedium && cfg.TaskMedium < cfg.TaskLong && cfg.TaskLong < cfg.TaskDeep) {
		return fmt.Errorf("task presets must be strictly increasing: short < medium < long < deep")
	}
	if cfg.IdleWarnAfter >= cfg.IdleLockAfter {
		return fmt.Errorf("idle.warn_after must be less than idle.lock_after")
	}
	if cfg.BreakWarning >= cfg.BreakLongStart {
		return fmt.Errorf("break.warning must be less than break.long_start")
	}
	if cfg.BreakWarning >= cfg.BreakDeepStart {
		return fmt.Errorf("break.warning must be less than break.deep_start")
	}
	if cfg.BreakLongStart >= cfg.TaskLong {
		return fmt.Errorf("break.long_start must be less than task.long")
	}
	if cfg.BreakDeepStart >= cfg.TaskDeep {
		return fmt.Errorf("break.deep_start must be less than task.deep")
	}
	if cfg.BreakLongStart+cfg.BreakLongDuration >= cfg.TaskLong {
		return fmt.Errorf("break.long_start + break.long_duration must be less than task.long")
	}
	if cfg.BreakDeepStart+cfg.BreakDeepDuration >= cfg.TaskDeep {
		return fmt.Errorf("break.deep_start + break.deep_duration must be less than task.deep")
	}
	if cfg.BreakRelockDelay >= cfg.BreakLongDuration {
		return fmt.Errorf("break.relock_delay must be less than break.long_duration")
	}
	if cfg.BreakRelockDelay >= cfg.BreakDeepDuration {
		return fmt.Errorf("break.relock_delay must be less than break.deep_duration")
	}

	return nil
}
