package protocol

import "time"

type Request struct {
	Command    string
	Start      *StartRequest
	HistoryAll bool
}

type StartRequest struct {
	Title    string
	Duration time.Duration
	Preset   string
	NoBreak  bool
}
