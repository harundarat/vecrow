// Package repository implements the ports declared by the use cases on top
// of the generic infrastructure interfaces (cache.Store, database.Querier).
//
// PostgreSQL queries go in queries/*.sql; `make sqlc` generates type-safe
// code into dbgen/.
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

const nonceKeyPrefix = "auth:nonce:"

// ErrNonceExists is returned when a nonce value is already stored.
var ErrNonceExists = errors.New("nonce already exists")

// NonceStore stores login nonces in the cache, keyed by nonce value, so the
// verify step can fetch and delete one atomically (GetDel).
type NonceStore struct {
	cache cache.Store
}

var _ auth.NonceStore = (*NonceStore)(nil)

func NewNonceStore(c cache.Store) *NonceStore {
	return &NonceStore{cache: c}
}

type nonceRecord struct {
	Scheme  string `json:"scheme"`
	Address string `json:"address"`
}

func (s *NonceStore) Save(ctx context.Context, n domain.Nonce, ttl time.Duration) error {
	b, err := json.Marshal(nonceRecord{Scheme: string(n.Wallet.Scheme), Address: n.Wallet.Address})
	if err != nil {
		return fmt.Errorf("nonce store: encode: %w", err)
	}
	stored, err := s.cache.SetNX(ctx, nonceKeyPrefix+n.Value, b, ttl)
	if err != nil {
		return fmt.Errorf("nonce store: %w", err)
	}
	if !stored {
		return fmt.Errorf("nonce store: %w", ErrNonceExists)
	}
	return nil
}
