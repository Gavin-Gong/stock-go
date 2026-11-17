// Package sdkerrors defines errors shared by all stock-go packages.
package sdkerrors

import "errors"

type Code string

const (
	Network         Code = "NETWORK_ERROR"
	Timeout         Code = "TIMEOUT"
	Aborted         Code = "ABORTED"
	HTTP            Code = "HTTP_ERROR"
	RateLimited     Code = "RATE_LIMITED"
	CircuitOpen     Code = "CIRCUIT_OPEN"
	UpstreamEmpty   Code = "UPSTREAM_EMPTY"
	UpstreamError   Code = "UPSTREAM_ERROR"
	Parse           Code = "PARSE_ERROR"
	InvalidSymbol   Code = "INVALID_SYMBOL"
	InvalidArgument Code = "INVALID_ARGUMENT"
	NotFound        Code = "NOT_FOUND"
)

// Error supports errors.As, errors.Is and unwrapping the original cause.
type Error struct {
	Code     Code
	Message  string
	Provider string
	URL      string
	Status   int
	Cause    error
}

func (e *Error) Error() string { return string(e.Code) + ": " + e.Message }
func (e *Error) Unwrap() error { return e.Cause }
func CodeOf(err error) Code {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}
