package protocol


import (
	"encoding/gob"
)

type Response struct {
	Type string
	Payload any
}

type ErrorResponse struct {
	Message string
}

func init() {
	gob.Register(ErrorResponse{})
}
