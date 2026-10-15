package tunnel

import (
	"lsyltunnel/src/internal/protocol"
)

func responseError(resp protocol.OpenResponse, fallback string) error {
	message := resp.Message
	if message == "" {
		message = fallback
	}
	return &protocol.ResponseError{Code: resp.Code, Message: message}
}
