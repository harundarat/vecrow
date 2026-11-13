// Package cache provides a key-value store with per-key expiry for
// short-lived state (login nonces, rate limits, ...).
//
// Callers depend on Store only; the Redis implementation is in redis.go.
package cache

import (
	"context"
	"errors"
	"time"
)

// Name is the component name used in logs and readiness checks.
const Name = "cache"

// ErrNotFound is returned when a key does not exist or has expired.
var ErrNotFound = errors.New("cache: key not found")

// Store is a key-value store. A ttl of 0 means the key never expires.
type Store interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	// SetNX stores value only if key does not exist and reports whether it did.
	SetNX(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error)
	// GetDel returns the value and deletes the key atomically.
	GetDel(ctx context.Context, key string) ([]byte, error)
	// Incr adds 1 to the counter at key and returns the new count. A new
	// counter expires after ttl; later increments do not extend it.
	Incr(ctx context.Context, key string, ttl time.Duration) (int64, error)
	Delete(ctx context.Context, keys ...string) error
}
