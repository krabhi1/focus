package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type File struct {
	Task               taskJSON     `json:"task"`
	Cooldown           cooldownJSON `json:"cooldown"`
	Break              breakJSON    `json:"break"`
	Idle               idleJSON     `json:"idle"`
	Alert              alertJSON    `json:"alert"`
	RelockDelay        string       `json:"relock_delay"`
	CooldownStartDelay string       `json:"cooldown_start_delay"`
}

type taskJSON struct {
	Short         string `json:"short"`
	Medium        string `json:"medium"`
	Long          string `json:"long"`
	Deep          string `json:"deep"`
	LongEndAction string `json:"long_end_action"`
	DeepEndAction string `json:"deep_end_action"`
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
}

type idleJSON struct {
	WarnAfter string `json:"warn_after"`
	LockAfter string `json:"lock_after"`
}

type alertJSON struct {
	RepeatCount *int `json:"repeat_count"`
}

type Overrides struct {
	TaskShort         *time.Duration
	TaskMedium        *time.Duration
	TaskLong          *time.Duration
	TaskDeep          *time.Duration
	TaskLongEndAction *string
	TaskDeepEndAction *string

	CooldownShort *time.Duration
	CooldownLong  *time.Duration
	CooldownDeep  *time.Duration

	BreakLongStart     *time.Duration
	BreakDeepStart     *time.Duration
	BreakWarning       *time.Duration
	BreakLongDuration  *time.Duration
	BreakDeepDuration  *time.Duration
	RelockDelay        *time.Duration
	CooldownStartDelay *time.Duration
	IdleWarnAfter      *time.Duration
	IdleLockAfter      *time.Duration

	CompletionAlertRepeatCount *int
}

type RuntimeConfig struct {
	TaskShort         time.Duration
	TaskMedium        time.Duration
	TaskLong          time.Duration
	TaskDeep          time.Duration
	TaskLongEndAction string
	TaskDeepEndAction string

	CooldownShort time.Duration
	CooldownLong  time.Duration
	CooldownDeep  time.Duration

	BreakLongStart     time.Duration
	BreakDeepStart     time.Duration
	BreakWarning       time.Duration
	BreakLongDuration  time.Duration
	BreakDeepDuration  time.Duration
	RelockDelay        time.Duration
	CooldownStartDelay time.Duration

	IdleWarnAfter              time.Duration
	IdleLockAfter              time.Duration
	CompletionAlertRepeatCount int
}

const (
	TaskShortDuration  = 15 * time.Minute
	TaskMediumDuration = 30 * time.Minute
	TaskLongDuration   = 60 * time.Minute
	TaskDeepDuration   = 90 * time.Minute

	TaskEndActionSleep = "sleep"
	TaskEndActionLock  = "lock"

	CooldownShortDuration = 5 * time.Minute
	CooldownLongDuration  = 10 * time.Minute
	CooldownDeepDuration  = 15 * time.Minute

	LongTaskBreakStartOffset = 25 * time.Minute
	DeepTaskBreakStartOffset = 45 * time.Minute
	BreakWarningOffset       = 2 * time.Minute
	RelockDelay              = 5 * time.Second
	CooldownStartDelay       = 2 * time.Minute

	IdleWarningAfter = 30 * time.Second
	IdleLockAfter    = 2 * time.Minute

	TaskLockedWaitDuration = 1 * time.Minute
)

var (
	runtimeConfigMu sync.RWMutex
	runtimeConfig   = DefaultRuntimeConfig()
)

func DefaultRuntimeConfig() RuntimeConfig {
	return RuntimeConfig{
		TaskShort:         TaskShortDuration,
		TaskMedium:        TaskMediumDuration,
		TaskLong:          TaskLongDuration,
		TaskDeep:          TaskDeepDuration,
		TaskLongEndAction: TaskEndActionLock,
		TaskDeepEndAction: TaskEndActionSleep,

		CooldownShort: CooldownShortDuration,
		CooldownLong:  CooldownLongDuration,
		CooldownDeep:  CooldownDeepDuration,

		BreakLongStart:     LongTaskBreakStartOffset,
		BreakDeepStart:     DeepTaskBreakStartOffset,
		BreakWarning:       BreakWarningOffset,
		BreakLongDuration:  5 * time.Minute,
		BreakDeepDuration:  5 * time.Minute,
		RelockDelay:        RelockDelay,
		CooldownStartDelay: CooldownStartDelay,

		IdleWarnAfter:              IdleWarningAfter,
		IdleLockAfter:              IdleLockAfter,
		CompletionAlertRepeatCount: 3,
	}
}

func SetRuntimeConfig(cfg RuntimeConfig) error {
	if err := ValidateRuntimeConfig(cfg); err != nil {
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

func ValidateRuntimeConfig(cfg RuntimeConfig) error {
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
		{"cooldown_start_delay", cfg.CooldownStartDelay},
		{"idle.warn_after", cfg.IdleWarnAfter},
		{"idle.lock_after", cfg.IdleLockAfter},
	}
	for _, item := range positive {
		if item.value <= 0 {
			return fmt.Errorf("%s must be > 0", item.name)
		}
	}
	if cfg.CompletionAlertRepeatCount < 0 {
		return fmt.Errorf("alert.repeat_count must be >= 0")
	}
	if cfg.RelockDelay < 0 {
		return fmt.Errorf("relock_delay must be >= 0")
	}
	if !isValidTaskEndAction(cfg.TaskLongEndAction) {
		return fmt.Errorf("task.long_end_action must be one of %q or %q", TaskEndActionSleep, TaskEndActionLock)
	}
	if !isValidTaskEndAction(cfg.TaskDeepEndAction) {
		return fmt.Errorf("task.deep_end_action must be one of %q or %q", TaskEndActionSleep, TaskEndActionLock)
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
	if cfg.RelockDelay >= cfg.BreakLongDuration {
		return fmt.Errorf("relock_delay must be less than break.long_duration")
	}
	if cfg.RelockDelay >= cfg.BreakDeepDuration {
		return fmt.Errorf("relock_delay must be less than break.deep_duration")
	}
	return nil
}

func SupportedConfigKeys() []string {
	return []string{
		"task.short",
		"task.medium",
		"task.long",
		"task.deep",
		"task.long_end_action",
		"task.deep_end_action",
		"cooldown.short",
		"cooldown.long",
		"cooldown.deep",
		"break.long_start",
		"break.deep_start",
		"break.warning",
		"break.long_duration",
		"break.deep_duration",
		"relock_delay",
		"cooldown_start_delay",
		"idle.warn_after",
		"idle.lock_after",
		"alert.repeat_count",
	}
}

func DescribeConfigKey(cfg RuntimeConfig, key string) (string, error) {
	switch key {
	case "task.short":
		return cfg.TaskShort.String(), nil
	case "task.medium":
		return cfg.TaskMedium.String(), nil
	case "task.long":
		return cfg.TaskLong.String(), nil
	case "task.deep":
		return cfg.TaskDeep.String(), nil
	case "task.long_end_action":
		return cfg.TaskLongEndAction, nil
	case "task.deep_end_action":
		return cfg.TaskDeepEndAction, nil
	case "cooldown.short":
		return cfg.CooldownShort.String(), nil
	case "cooldown.long":
		return cfg.CooldownLong.String(), nil
	case "cooldown.deep":
		return cfg.CooldownDeep.String(), nil
	case "break.long_start":
		return cfg.BreakLongStart.String(), nil
	case "break.deep_start":
		return cfg.BreakDeepStart.String(), nil
	case "break.warning":
		return cfg.BreakWarning.String(), nil
	case "break.long_duration":
		return cfg.BreakLongDuration.String(), nil
	case "break.deep_duration":
		return cfg.BreakDeepDuration.String(), nil
	case "relock_delay":
		return cfg.RelockDelay.String(), nil
	case "cooldown_start_delay":
		return cfg.CooldownStartDelay.String(), nil
	case "idle.warn_after":
		return cfg.IdleWarnAfter.String(), nil
	case "idle.lock_after":
		return cfg.IdleLockAfter.String(), nil
	case "alert.repeat_count":
		return strconv.Itoa(cfg.CompletionAlertRepeatCount), nil
	default:
		return "", fmt.Errorf("unknown config key %q", key)
	}
}

func UpdateConfigValue(path, key, rawValue string) error {
	raw, _, err := loadConfigMap(path)
	if err != nil {
		return err
	}
	if raw == nil {
		raw = map[string]any{}
	}

	if err := applyConfigValue(raw, key, rawValue); err != nil {
		return err
	}

	fileCfg, err := configMapToFile(raw)
	if err != nil {
		return err
	}
	resolved, err := ResolveRuntimeConfig(DefaultRuntimeConfig(), fileCfg, Overrides{})
	if err != nil {
		return err
	}
	if err := ValidateRuntimeConfig(resolved); err != nil {
		return err
	}
	return writeConfigMap(path, raw)
}

func DefaultPath() (string, error) {
	if p := os.Getenv("FOCUS_CONFIG"); p != "" {
		return p, nil
	}
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
	if err := rejectLegacyBreakRelockDelay(data); err != nil {
		return File{}, true, err
	}
	if err := rejectLegacyAlertRepeatInterval(data); err != nil {
		return File{}, true, err
	}
	var cfg File
	if err := json.Unmarshal(data, &cfg); err != nil {
		return File{}, true, fmt.Errorf("parse config JSON: %w", err)
	}
	return cfg, true, nil
}

func loadConfigMap(path string) (map[string]any, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]any{}, false, nil
		}
		return nil, false, fmt.Errorf("read config: %w", err)
	}
	if err := rejectLegacyBreakRelockDelay(data); err != nil {
		return nil, true, err
	}
	if err := rejectLegacyAlertRepeatInterval(data); err != nil {
		return nil, true, err
	}
	var raw map[string]any
	if len(data) == 0 {
		return map[string]any{}, true, nil
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, true, fmt.Errorf("parse config JSON: %w", err)
	}
	if raw == nil {
		raw = map[string]any{}
	}
	return raw, true, nil
}

func configMapToFile(raw map[string]any) (File, error) {
	data, err := json.Marshal(raw)
	if err != nil {
		return File{}, fmt.Errorf("marshal config JSON: %w", err)
	}
	var cfg File
	if err := json.Unmarshal(data, &cfg); err != nil {
		return File{}, fmt.Errorf("parse config JSON: %w", err)
	}
	return cfg, nil
}

func ResolveRuntimeConfig(defaults RuntimeConfig, fileCfg File, overrides Overrides) (RuntimeConfig, error) {
	resolved := defaults
	if err := applyDuration(&resolved.TaskShort, fileCfg.Task.Short); err != nil {
		return RuntimeConfig{}, fmt.Errorf("invalid task.short: %w", err)
	}
	if err := applyDuration(&resolved.TaskMedium, fileCfg.Task.Medium); err != nil {
		return RuntimeConfig{}, fmt.Errorf("invalid task.medium: %w", err)
	}
	if err := applyDuration(&resolved.TaskLong, fileCfg.Task.Long); err != nil {
		return RuntimeConfig{}, fmt.Errorf("invalid task.long: %w", err)
	}
	if err := applyDuration(&resolved.TaskDeep, fileCfg.Task.Deep); err != nil {
		return RuntimeConfig{}, fmt.Errorf("invalid task.deep: %w", err)
	}
	if err := applyTaskEndAction(&resolved.TaskLongEndAction, fileCfg.Task.LongEndAction); err != nil {
		return RuntimeConfig{}, fmt.Errorf("invalid task.long_end_action: %w", err)
	}
	if err := applyTaskEndAction(&resolved.TaskDeepEndAction, fileCfg.Task.DeepEndAction); err != nil {
		return RuntimeConfig{}, fmt.Errorf("invalid task.deep_end_action: %w", err)
	}
	if err := applyDuration(&resolved.CooldownShort, fileCfg.Cooldown.Short); err != nil {
		return RuntimeConfig{}, fmt.Errorf("invalid cooldown.short: %w", err)
	}
	if err := applyDuration(&resolved.CooldownLong, fileCfg.Cooldown.Long); err != nil {
		return RuntimeConfig{}, fmt.Errorf("invalid cooldown.long: %w", err)
	}
	if err := applyDuration(&resolved.CooldownDeep, fileCfg.Cooldown.Deep); err != nil {
		return RuntimeConfig{}, fmt.Errorf("invalid cooldown.deep: %w", err)
	}
	if err := applyDuration(&resolved.BreakLongStart, fileCfg.Break.LongStart); err != nil {
		return RuntimeConfig{}, fmt.Errorf("invalid break.long_start: %w", err)
	}
	if err := applyDuration(&resolved.BreakDeepStart, fileCfg.Break.DeepStart); err != nil {
		return RuntimeConfig{}, fmt.Errorf("invalid break.deep_start: %w", err)
	}
	if err := applyDuration(&resolved.BreakWarning, fileCfg.Break.Warning); err != nil {
		return RuntimeConfig{}, fmt.Errorf("invalid break.warning: %w", err)
	}
	if err := applyDuration(&resolved.BreakLongDuration, fileCfg.Break.LongDuration); err != nil {
		return RuntimeConfig{}, fmt.Errorf("invalid break.long_duration: %w", err)
	}
	if err := applyDuration(&resolved.BreakDeepDuration, fileCfg.Break.DeepDuration); err != nil {
		return RuntimeConfig{}, fmt.Errorf("invalid break.deep_duration: %w", err)
	}
	if err := applyDuration(&resolved.RelockDelay, fileCfg.RelockDelay); err != nil {
		return RuntimeConfig{}, fmt.Errorf("invalid relock_delay: %w", err)
	}
	if err := applyDuration(&resolved.CooldownStartDelay, fileCfg.CooldownStartDelay); err != nil {
		return RuntimeConfig{}, fmt.Errorf("invalid cooldown_start_delay: %w", err)
	}
	if err := applyDuration(&resolved.IdleWarnAfter, fileCfg.Idle.WarnAfter); err != nil {
		return RuntimeConfig{}, fmt.Errorf("invalid idle.warn_after: %w", err)
	}
	if err := applyDuration(&resolved.IdleLockAfter, fileCfg.Idle.LockAfter); err != nil {
		return RuntimeConfig{}, fmt.Errorf("invalid idle.lock_after: %w", err)
	}
	if fileCfg.Alert.RepeatCount != nil {
		resolved.CompletionAlertRepeatCount = *fileCfg.Alert.RepeatCount
	}
	applyOverride(&resolved.TaskShort, overrides.TaskShort)
	applyOverride(&resolved.TaskMedium, overrides.TaskMedium)
	applyOverride(&resolved.TaskLong, overrides.TaskLong)
	applyOverride(&resolved.TaskDeep, overrides.TaskDeep)
	applyOverrideString(&resolved.TaskLongEndAction, overrides.TaskLongEndAction)
	applyOverrideString(&resolved.TaskDeepEndAction, overrides.TaskDeepEndAction)
	applyOverride(&resolved.CooldownShort, overrides.CooldownShort)
	applyOverride(&resolved.CooldownLong, overrides.CooldownLong)
	applyOverride(&resolved.CooldownDeep, overrides.CooldownDeep)
	applyOverride(&resolved.BreakLongStart, overrides.BreakLongStart)
	applyOverride(&resolved.BreakDeepStart, overrides.BreakDeepStart)
	applyOverride(&resolved.BreakWarning, overrides.BreakWarning)
	applyOverride(&resolved.BreakLongDuration, overrides.BreakLongDuration)
	applyOverride(&resolved.BreakDeepDuration, overrides.BreakDeepDuration)
	applyOverride(&resolved.RelockDelay, overrides.RelockDelay)
	applyOverride(&resolved.CooldownStartDelay, overrides.CooldownStartDelay)
	applyOverride(&resolved.IdleWarnAfter, overrides.IdleWarnAfter)
	applyOverride(&resolved.IdleLockAfter, overrides.IdleLockAfter)
	applyOverrideInt(&resolved.CompletionAlertRepeatCount, overrides.CompletionAlertRepeatCount)
	return resolved, nil
}

func applyDuration(dst *time.Duration, raw string) error {
	if raw == "" {
		return nil
	}
	value, err := time.ParseDuration(raw)
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

func applyOverrideInt(dst *int, value *int) {
	if value != nil {
		*dst = *value
	}
}

func applyOverrideString(dst *string, value *string) {
	if value != nil {
		*dst = strings.ToLower(strings.TrimSpace(*value))
	}
}

func applyTaskEndAction(dst *string, raw string) error {
	if raw == "" {
		return nil
	}
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if !isValidTaskEndAction(normalized) {
		return fmt.Errorf("must be one of %q or %q", TaskEndActionSleep, TaskEndActionLock)
	}
	*dst = normalized
	return nil
}

func isValidTaskEndAction(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case TaskEndActionSleep, TaskEndActionLock:
		return true
	default:
		return false
	}
}

func applyConfigValue(raw map[string]any, key, rawValue string) error {
	section, leaf, ok := strings.Cut(key, ".")
	if !ok {
		switch key {
		case "relock_delay", "cooldown_start_delay":
			raw[key] = rawValue
			return nil
		default:
			return fmt.Errorf("unknown config key %q", key)
		}
	}

	sectionMap, _ := raw[section].(map[string]any)
	if sectionMap == nil {
		sectionMap = map[string]any{}
		raw[section] = sectionMap
	}

	switch section {
	case "task", "cooldown", "break", "idle", "alert":
		switch leaf {
		case "short", "medium", "long", "deep", "long_start", "deep_start", "warning", "long_duration", "deep_duration", "warn_after", "lock_after":
			sectionMap[leaf] = rawValue
			return nil
		case "long_end_action", "deep_end_action":
			if !isValidTaskEndAction(rawValue) {
				return fmt.Errorf("task.%s must be one of %q or %q", leaf, TaskEndActionSleep, TaskEndActionLock)
			}
			sectionMap[leaf] = strings.ToLower(strings.TrimSpace(rawValue))
			return nil
		case "repeat_count":
			value, err := strconv.Atoi(rawValue)
			if err != nil {
				return fmt.Errorf("invalid config value %q for %s: %w", rawValue, key, err)
			}
			if value < 0 {
				return fmt.Errorf("alert.repeat_count must be >= 0")
			}
			sectionMap[leaf] = value
			return nil
		default:
			return fmt.Errorf("unknown config key %q", key)
		}
	default:
		return fmt.Errorf("unknown config key %q", key)
	}
}

func writeConfigMap(path string, raw map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config JSON: %w", err)
	}
	data = append(data, '\n')

	dir := filepath.Dir(path)
	base := filepath.Base(path)
	tmp, err := os.CreateTemp(dir, "."+base+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp config: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace config file: %w", err)
	}
	return nil
}

func rejectLegacyBreakRelockDelay(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}
	breakRaw, ok := raw["break"]
	if !ok {
		return nil
	}
	var breakFields map[string]json.RawMessage
	if err := json.Unmarshal(breakRaw, &breakFields); err != nil {
		return nil
	}
	if _, ok := breakFields["relock_delay"]; ok {
		return fmt.Errorf("legacy break.relock_delay is not supported; use top-level relock_delay")
	}
	return nil
}

func rejectLegacyAlertRepeatInterval(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}
	alertRaw, ok := raw["alert"]
	if !ok {
		return nil
	}
	var alertFields map[string]json.RawMessage
	if err := json.Unmarshal(alertRaw, &alertFields); err != nil {
		return nil
	}
	if _, ok := alertFields["repeat_interval"]; ok {
		return fmt.Errorf("legacy alert.repeat_interval is not supported; use alert.repeat_count")
	}
	return nil
}
