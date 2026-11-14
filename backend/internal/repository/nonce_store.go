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
	Scheme    string    `json:"scheme"`
	Address   string    `json:"address"`
	ExpiresAt time.Time `json:"expires_at"`
	// Message is stored as issued, so verification checks exactly the text
	// the wallet was asked to sign.
	Message string `json:"message"`
}

func (s *NonceStore) Save(ctx context.Context, n domain.Nonce, ttl time.Duration) error {
	b, err := json.Marshal(nonceRecord{
		Scheme:    string(n.Wallet.Scheme),
		Address:   n.Wallet.Address,
		ExpiresAt: n.ExpiresAt,
		Message:   n.Message,
	})
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

func (s *NonceStore) Consume(ctx context.Context, value string) (domain.Nonce, error) {
	b, err := s.cache.GetDel(ctx, nonceKeyPrefix+value)
	if errors.Is(err, cache.ErrNotFound) {
		return domain.Nonce{}, auth.ErrNonceNotFound
	}
	if err != nil {
		return domain.Nonce{}, fmt.Errorf("nonce store: %w", err)
	}
	var rec nonceRecord
	if err := json.Unmarshal(b, &rec); err != nil {
		return domain.Nonce{}, fmt.Errorf("nonce store: decode: %w", err)
	}
	return domain.Nonce{
		Value:     value,
		Wallet:    domain.Wallet{Scheme: domain.WalletScheme(rec.Scheme), Address: rec.Address},
		ExpiresAt: rec.ExpiresAt,
		Message:   rec.Message,
	}, nil
}
