package main

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

func TestLogStartupVersion(t *testing.T) {
	oldVersion := version
	version = "v1.2.3"
	t.Cleanup(func() {
		version = oldVersion
	})

	var buf bytes.Buffer
	prevWriter := log.Writer()
	prevFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(prevWriter)
		log.SetFlags(prevFlags)
	})

	logStartupVersion()

	got := buf.String()
	if !strings.Contains(got, "Focus daemon version v1.2.3") {
		t.Fatalf("log output = %q, want version line", got)
	}
}
