package events

import "time"

type Kind string

const (
	KindListener Kind = "listener"
	KindIdle     Kind = "idle"
	KindScreen   Kind = "screen"
	KindSleep    Kind = "sleep"
	KindShutdown Kind = "shutdown"
)

type Event struct {
	Timestamp time.Time
	Kind      Kind
	State     string
	Fields    map[string]string
}
