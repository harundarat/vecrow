// Package requestid carries the correlation ID of a request across HTTP and
// queue boundaries.
package requestid

import (
	"context"

	"github.com/google/uuid"
)

// Header is the HTTP header (and queue message header) holding the ID.
const Header = "X-Request-ID"

const maxLen = 128

type ctxKey struct{}

// New returns a fresh random ID.
func New() string { return uuid.NewString() }

// With returns a copy of ctx carrying id.
func With(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// From returns the ID stored in ctx, or "".
func From(ctx context.Context) string {
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}

// Valid reports whether an ID received from a client is safe to reuse:
// 1–128 characters of [A-Za-z0-9._:-]. Anything else is replaced, which also
// prevents log injection through the header.
func Valid(id string) bool {
	if id == "" || len(id) > maxLen {
		return false
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-', r == '_', r == '.', r == ':':
		default:
			return false
		}
	}
	return true
}
