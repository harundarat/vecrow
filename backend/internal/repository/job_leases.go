package repository

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/harundarat/vecrow/backend/internal/infrastructure/cache"
	"github.com/harundarat/vecrow/backend/internal/usecase/jobs"
)

// JobLeases keeps job leases and escrow locks in the cache: a key held with
// a random token, set only if absent.
type JobLeases struct {
	cache cache.Store
}

var _ jobs.Leases = (*JobLeases)(nil)

func NewJobLeases(c cache.Store) *JobLeases { return &JobLeases{cache: c} }

func (l *JobLeases) Acquire(ctx context.Context, key string, ttl time.Duration) (string, bool, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", false, fmt.Errorf("job leases: token: %w", err)
	}
	token := hex.EncodeToString(b)
	ok, err := l.cache.SetNX(ctx, key, []byte(token), ttl)
	if err != nil {
		return "", false, fmt.Errorf("job leases: %w", err)
	}
	if !ok {
		return "", false, nil
	}
	return token, true, nil
}

// Release deletes key if it holds token, or whatever it holds when token is
// empty. The check and the delete are separate calls: a lease that expired
// in between and was taken again may be freed early. Leases only avoid
// duplicate work, which the jobs tolerate.
func (l *JobLeases) Release(ctx context.Context, key, token string) error {
	if token != "" {
		held, err := l.cache.Get(ctx, key)
		if errors.Is(err, cache.ErrNotFound) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("job leases: %w", err)
		}
		if string(held) != token {
			return nil
		}
	}
	if err := l.cache.Delete(ctx, key); err != nil {
		return fmt.Errorf("job leases: %w", err)
	}
	return nil
}
