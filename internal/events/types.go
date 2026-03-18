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

const (
	wireMagic0  = 'F'
	wireMagic1  = 'E'
	wireMagic2  = 'V'
	wireVersion = 1
	wireSize    = 24
)

const (
	wireKindListener = 1
	wireKindIdle     = 2
	wireKindScreen   = 3
	wireKindSleep    = 4
	wireKindShutdown = 5
)

const (
	wireStateNone      = 0
	wireStateReady     = 1
	wireStateEntered   = 2
	wireStateExited    = 3
	wireStateLocked    = 4
	wireStateUnlocked  = 5
	wireStatePrepare   = 6
	wireStateResume    = 7
	wireStateCancelled = 8
)

type Event struct {
	Timestamp time.Time
	Kind      Kind
	State     string
	Fields    map[string]string
}
