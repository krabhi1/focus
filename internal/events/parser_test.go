package events

import (
	"encoding/binary"
	"testing"
	"time"
)

func TestParseBinaryFrameListener(t *testing.T) {
	ts := time.Date(2026, 3, 18, 20, 30, 0, 0, time.UTC)
	frame := buildFrame(
		wireKindListener,
		wireStateReady,
		uint64(ts.UnixMilli()),
		300,
		1,
	)

	event, err := ParseBinaryFrame(frame)
	if err != nil {
		t.Fatalf("ParseBinaryFrame returned error: %v", err)
	}

	if event.Kind != KindListener {
		t.Fatalf("Kind = %q, want %q", event.Kind, KindListener)
	}
	if event.State != "ready" {
		t.Fatalf("State = %q, want %q", event.State, "ready")
	}
	if !event.Timestamp.Equal(ts) {
		t.Fatalf("Timestamp = %v, want %v", event.Timestamp, ts)
	}
	if got := event.Fields["idle_threshold"]; got != "300" {
		t.Fatalf("idle_threshold = %q, want %q", got, "300")
	}
	if got := event.Fields["idle_poll"]; got != "1" {
		t.Fatalf("idle_poll = %q, want %q", got, "1")
	}
}

func TestParseBinaryFrameIdleEntered(t *testing.T) {
	ts := time.Date(2026, 3, 18, 20, 31, 0, 0, time.UTC)
	frame := buildFrame(
		wireKindIdle,
		wireStateEntered,
		uint64(ts.UnixMilli()),
		300,
		1,
	)

	event, err := ParseBinaryFrame(frame)
	if err != nil {
		t.Fatalf("ParseBinaryFrame returned error: %v", err)
	}

	if event.Kind != KindIdle {
		t.Fatalf("Kind = %q, want %q", event.Kind, KindIdle)
	}
	if event.State != "entered" {
		t.Fatalf("State = %q, want %q", event.State, "entered")
	}
	if _, ok := event.Fields["idle_threshold"]; ok {
		t.Fatalf("idle_threshold unexpectedly present for non-listener event")
	}
}

func TestParseBinaryFrameRejectsBadMagic(t *testing.T) {
	frame := buildFrame(wireKindIdle, wireStateEntered, 0, 300, 1)
	frame[0] = 'X'

	if _, err := ParseBinaryFrame(frame); err == nil {
		t.Fatal("expected error for invalid frame magic")
	}
}

func TestParseBinaryFrameRejectsBadSize(t *testing.T) {
	frame := buildFrame(wireKindIdle, wireStateEntered, 0, 300, 1)
	binary.LittleEndian.PutUint16(frame[6:8], 99)

	if _, err := ParseBinaryFrame(frame); err == nil {
		t.Fatal("expected error for invalid frame size")
	}
}

func buildFrame(kind, state byte, unixMillis uint64, idleThreshold, idlePoll uint32) []byte {
	frame := make([]byte, wireSize)
	frame[0] = wireMagic0
	frame[1] = wireMagic1
	frame[2] = wireMagic2
	frame[3] = wireVersion
	frame[4] = kind
	frame[5] = state
	binary.LittleEndian.PutUint16(frame[6:8], wireSize)
	binary.LittleEndian.PutUint64(frame[8:16], unixMillis)
	binary.LittleEndian.PutUint32(frame[16:20], idleThreshold)
	binary.LittleEndian.PutUint32(frame[20:24], idlePoll)
	return frame
}
