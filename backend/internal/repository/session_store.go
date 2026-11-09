package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/harundarat/vecrow/backend/internal/domain"
	"github.com/harundarat/vecrow/backend/internal/infrastructure/cache"
	"github.com/harundarat/vecrow/backend/internal/usecase/auth"
)

const sessionKeyPrefix = "auth:session:"

// SessionStore stores sessions in the cache with the session TTL. Keys are
// derived from the token by the use case; tokens are never stored.
type SessionStore struct {
	cache cache.Store
}

var _ auth.SessionStore = (*SessionStore)(nil)

func NewSessionStore(c cache.Store) *SessionStore {
	return &SessionStore{cache: c}
}

type sessionRecord struct {
	AccountID string    `json:"account_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (s *SessionStore) Save(ctx context.Context, key string, sess domain.Session, ttl time.Duration) error {
	b, err := json.Marshal(sessionRecord{AccountID: sess.AccountID, ExpiresAt: sess.ExpiresAt})
	if err != nil {
		return fmt.Errorf("session store: encode: %w", err)
	}
	if err := s.cache.Set(ctx, sessionKeyPrefix+key, b, ttl); err != nil {
		return fmt.Errorf("session store: %w", err)
	}
	return nil
}

func (s *SessionStore) Get(ctx context.Context, key string) (domain.Session, error) {
	b, err := s.cache.Get(ctx, sessionKeyPrefix+key)
	if errors.Is(err, cache.ErrNotFound) {
		return domain.Session{}, auth.ErrSessionNotFound
	}
	if err != nil {
		return domain.Session{}, fmt.Errorf("session store: %w", err)
	}
	var rec sessionRecord
	if err := json.Unmarshal(b, &rec); err != nil {
		return domain.Session{}, fmt.Errorf("session store: decode: %w", err)
	}
	return domain.Session{AccountID: rec.AccountID, ExpiresAt: rec.ExpiresAt}, nil
}

func (s *SessionStore) Delete(ctx context.Context, key string) error {
	if err := s.cache.Delete(ctx, sessionKeyPrefix+key); err != nil {
		return fmt.Errorf("session store: %w", err)
	}
	return nil
}
