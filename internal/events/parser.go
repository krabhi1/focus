package events

import (
	"fmt"
	"regexp"
	"strconv"
	"time"
)

var fieldPattern = regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_]*)=(".*?"|[^ ]+)`)

func Parse(line string) (Event, error) {
	fields := make(map[string]string)
	matches := fieldPattern.FindAllStringSubmatch(line, -1)
	if len(matches) == 0 {
		return Event{}, fmt.Errorf("invalid event line: %q", line)
	}

	for _, match := range matches {
		key := match[1]
		value := match[2]
		if unquoted, err := strconv.Unquote(value); err == nil {
			value = unquoted
		}
		fields[key] = value
	}

	eventName, ok := fields["event"]
	if !ok {
		return Event{}, fmt.Errorf("missing event field: %q", line)
	}

	var ts time.Time
	if rawTS, ok := fields["ts"]; ok {
		parsed, err := time.ParseInLocation("2006-01-02 15:04:05", rawTS, time.Local)
		if err != nil {
			return Event{}, fmt.Errorf("invalid timestamp %q: %w", rawTS, err)
		}
		ts = parsed
	}

	return Event{
		Timestamp: ts,
		Kind:      Kind(eventName),
		State:     fields["state"],
		Fields:    fields,
	}, nil
}
