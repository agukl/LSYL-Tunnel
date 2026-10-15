package protocol

// ResponseError preserves a rejected response's machine-readable code.
// Error retains the historical text format for logs and older consumers.
type ResponseError struct {
	Code    string
	Message string
}

func (e *ResponseError) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}
