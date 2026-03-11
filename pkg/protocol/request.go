package protocol

import (
	"encoding/gob"
	"time"
)

type Request struct {
	Command string
	Payload any
}

type StartRequest struct {
	Title    string
	Duration time.Duration
}

func init() {
	gob.Register(StartRequest{})
}
