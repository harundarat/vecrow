// Package domain holds Vecrow's entities, value objects and business rules.
// It has no dependencies outside the standard library.
package domain

import (
	"errors"
	"fmt"
)

// Error kinds. Outer layers map them (for example to HTTP status codes) with
// errors.Is, without knowing where the error came from.
var (
	ErrInvalidArgument = errors.New("invalid argument")
	ErrNotFound        = errors.New("not found")
	ErrConflict        = errors.New("conflict")
	ErrUnauthorized    = errors.New("unauthorized")
	ErrForbidden       = errors.New("forbidden")
	// ErrUnavailable: a dependency such as the NEAR chain did not answer in
	// time; the request may be retried.
	ErrUnavailable = errors.New("unavailable")
	// ErrTooManyRequests: a per-subject limit (such as one-time codes per
	// email) was reached; the request may be retried later.
	ErrTooManyRequests = errors.New("too many requests")
)

// Error is a business error with a message that is safe to show to clients.
type Error struct {
	Kind    error
	Message string
}

func (e *Error) Error() string { return e.Message }
func (e *Error) Unwrap() error { return e.Kind }

func newError(kind error, format string, args ...any) error {
	return &Error{Kind: kind, Message: fmt.Sprintf(format, args...)}
}

func InvalidArgument(format string, args ...any) error {
	return newError(ErrInvalidArgument, format, args...)
}

func NotFound(format string, args ...any) error { return newError(ErrNotFound, format, args...) }

func Conflict(format string, args ...any) error { return newError(ErrConflict, format, args...) }

func Unauthorized(format string, args ...any) error {
	return newError(ErrUnauthorized, format, args...)
}

func Forbidden(format string, args ...any) error { return newError(ErrForbidden, format, args...) }

func Unavailable(format string, args ...any) error { return newError(ErrUnavailable, format, args...) }

func TooManyRequests(format string, args ...any) error {
	return newError(ErrTooManyRequests, format, args...)
}
