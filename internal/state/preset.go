package state

import (
	"fmt"
	"strings"
	"time"
)

func ResolveTaskPresetDuration(name string) (time.Duration, error) {
	cfg := GetRuntimeConfig()
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "short":
		return cfg.TaskShort, nil
	case "medium":
		return cfg.TaskMedium, nil
	case "long":
		return cfg.TaskLong, nil
	case "deep":
		return cfg.TaskDeep, nil
	default:
		return 0, fmt.Errorf("unknown duration preset %q, choose one of short|medium|long|deep", name)
	}
}
