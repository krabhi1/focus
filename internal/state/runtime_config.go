package state

import (
	"fmt"
	"sync"
	"time"
)

type RuntimeConfig struct {
	CooldownShort time.Duration
	CooldownLong  time.Duration
	CooldownDeep  time.Duration

	BreakLongStart    time.Duration
	BreakDeepStart    time.Duration
	BreakWarning      time.Duration
	BreakLongDuration time.Duration
	BreakDeepDuration time.Duration
	BreakRelockDelay  time.Duration

	IdleWarnAfter    time.Duration
	IdleLockAfter    time.Duration
	IdlePollInterval time.Duration
}

var (
	runtimeConfigMu sync.RWMutex
	runtimeConfig   = DefaultRuntimeConfig()
)

func DefaultRuntimeConfig() RuntimeConfig {
	return RuntimeConfig{
		CooldownShort: ShortCooldownDuration,
		CooldownLong:  LongCooldownDuration,
		CooldownDeep:  DeepCooldownDuration,

		BreakLongStart:    LongTaskBreakStartOffset,
		BreakDeepStart:    DeepTaskBreakStartOffset,
		BreakWarning:      BreakWarningOffset,
		BreakLongDuration: 5 * time.Minute,
		BreakDeepDuration: 10 * time.Minute,
		BreakRelockDelay:  BreakRelockDelay,

		IdleWarnAfter:    IdleWarningAfter,
		IdleLockAfter:    IdleLockAfter,
		IdlePollInterval: IdleMonitorInterval,
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
	}

	for _, item := range positive {
		if item.value <= 0 {
			return fmt.Errorf("%s must be > 0", item.name)
		}
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
	if cfg.BreakLongStart >= 60*time.Minute {
		return fmt.Errorf("break.long_start must be less than long task duration (60m)")
	}
	if cfg.BreakDeepStart >= 90*time.Minute {
		return fmt.Errorf("break.deep_start must be less than deep task duration (90m)")
	}

	return nil
}
