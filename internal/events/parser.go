package events

import (
	"encoding/binary"
	"fmt"
	"time"
)

func ParseBinaryFrame(frame []byte) (Event, error) {
	if len(frame) != wireSize {
		return Event{}, fmt.Errorf("invalid frame size: %d", len(frame))
	}
	if frame[0] != wireMagic0 || frame[1] != wireMagic1 || frame[2] != wireMagic2 {
		return Event{}, fmt.Errorf("invalid frame magic")
	}
	if frame[3] != wireVersion {
		return Event{}, fmt.Errorf("unsupported frame version: %d", frame[3])
	}
	if binary.LittleEndian.Uint16(frame[6:8]) != wireSize {
		return Event{}, fmt.Errorf("unexpected frame length: %d", binary.LittleEndian.Uint16(frame[6:8]))
	}
	fields := map[string]string{}
	kind, err := parseWireKind(frame[4])
	if err != nil {
		return Event{}, err
	}
	state, err := parseWireState(frame[5])
	if err != nil {
		return Event{}, err
	}
	unixMillis := binary.LittleEndian.Uint64(frame[8:16])
	ts := time.UnixMilli(int64(unixMillis))
	fields["event"] = string(kind)
	if state != "" {
		fields["state"] = state
	}
	return Event{Timestamp: ts, Kind: kind, State: state, Fields: fields}, nil
}

func parseWireKind(raw byte) (Kind, error) {
	switch raw {
	case wireKindListener:
		return KindListener, nil
	case wireKindScreen:
		return KindScreen, nil
	case wireKindSleep:
		return KindSleep, nil
	case wireKindShutdown:
		return KindShutdown, nil
	default:
		return "", fmt.Errorf("unknown wire kind: %d", raw)
	}
}

func parseWireState(raw byte) (string, error) {
	switch raw {
	case wireStateNone:
		return "", nil
	case wireStateReady:
		return "ready", nil
	case wireStateEntered:
		return "entered", nil
	case wireStateExited:
		return "exited", nil
	case wireStateLocked:
		return "locked", nil
	case wireStateUnlocked:
		return "unlocked", nil
	case wireStatePrepare:
		return "prepare", nil
	case wireStateResume:
		return "resume", nil
	case wireStateCancelled:
		return "cancelled", nil
	default:
		return "", fmt.Errorf("unknown wire state: %d", raw)
	}
}
