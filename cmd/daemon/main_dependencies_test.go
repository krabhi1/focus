package main

import (
	"bytes"
	"errors"
	"log"
	"strings"
	"testing"
)

func TestWarnMissingRuntimeDependencies(t *testing.T) {
	var calls []string
	lookPath := func(name string) (string, error) {
		calls = append(calls, name)
		if name == "notify-send" {
			return "", errors.New("not found")
		}
		return "/usr/bin/" + name, nil
	}

	var buf bytes.Buffer
	prevWriter := log.Writer()
	prevFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(prevWriter)
		log.SetFlags(prevFlags)
	}()

	warnMissingRuntimeDependencies(lookPath)

	if len(calls) != 3 {
		t.Fatalf("lookPath calls = %d, want 3", len(calls))
	}
	got := buf.String()
	if !strings.Contains(got, "missing dependency 'notify-send'") {
		t.Fatalf("log output = %q, want missing notify-send warning", got)
	}
	if strings.Contains(got, "xdg-screensaver") {
		t.Fatalf("log output = %q, did not expect warning for xdg-screensaver", got)
	}
	if strings.Contains(got, "paplay") {
		t.Fatalf("log output = %q, did not expect warning for paplay", got)
	}
}
