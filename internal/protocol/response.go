package protocol

type Response struct {
	Type    string
	Success *SuccessResponse
	Error   *ErrorResponse
}

type SuccessResponse struct {
	Message string
}

type ErrorResponse struct {
	Message string
}
