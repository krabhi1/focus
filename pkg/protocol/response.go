package protocol


import (
	"encoding/gob"
)

type Response struct {
	Type string
	Payload any
}
type SuccessResponse struct {
	Message string
}
type ErrorResponse struct {
	Message string
}

func init() {
	gob.Register(SuccessResponse{})
	gob.Register(ErrorResponse{})
}
