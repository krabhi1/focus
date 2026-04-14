package storage

import (
	"testing"
	"time"
)

func TestResolveTaskPresetDuration(t *testing.T) {
	t.Cleanup(func() {
		_ = SetRuntimeConfig(DefaultRuntimeConfig())
	})

	cfg := DefaultRuntimeConfig()
	cfg.TaskShort = 11 * time.Second
	cfg.TaskMedium = 22 * time.Second
	cfg.TaskLong = 33 * time.Second
	cfg.TaskDeep = 44 * time.Second
	cfg.BreakWarning = 2 * time.Second
	cfg.BreakLongStart = 12 * time.Second
	cfg.BreakDeepStart = 24 * time.Second
	cfg.BreakLongDuration = 6 * time.Second
	cfg.BreakDeepDuration = 10 * time.Second
	cfg.RelockDelay = 1 * time.Second
	cfg.CooldownStartDelay = 3 * time.Second
	if err := SetRuntimeConfig(cfg); err != nil {
		t.Fatalf("SetRuntimeConfig failed: %v", err)
	}

	tests := []struct {
		name   string
		preset string
		want   time.Duration
	}{
		{name: "short", preset: "short", want: 11 * time.Second},
		{name: "medium", preset: "medium", want: 22 * time.Second},
		{name: "long", preset: "long", want: 33 * time.Second},
		{name: "deep", preset: "deep", want: 44 * time.Second},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveTaskPresetDuration(tc.preset)
			if err != nil {
				t.Fatalf("ResolveTaskPresetDuration returned error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("ResolveTaskPresetDuration(%q) = %s, want %s", tc.preset, got, tc.want)
			}
		})
	}
}

func TestDefaultRuntimeConfigDefaults(t *testing.T) {
	cfg := DefaultRuntimeConfig()
	if cfg.RelockDelay != 10*time.Second {
		t.Fatalf("RelockDelay = %s, want 10s", cfg.RelockDelay)
	}
	if cfg.CooldownStartDelay != 2*time.Minute {
		t.Fatalf("CooldownStartDelay = %s, want 2m", cfg.CooldownStartDelay)
	}
	if cfg.IdleLockAfter != 2*time.Minute {
		t.Fatalf("IdleLockAfter = %s, want 2m", cfg.IdleLockAfter)
	}
	if cfg.BreakDeepDuration != 10*time.Minute {
		t.Fatalf("BreakDeepDuration = %s, want 10m", cfg.BreakDeepDuration)
	}
	if cfg.CompletionAlertRepeatCount != 3 {
		t.Fatalf("CompletionAlertRepeatCount = %d, want 3", cfg.CompletionAlertRepeatCount)
	}
}

func TestResolveTaskPresetDurationRejectsUnknown(t *testing.T) {
	if _, err := ResolveTaskPresetDuration("unknown"); err == nil {
		t.Fatal("expected error for unknown preset")
	}
}
