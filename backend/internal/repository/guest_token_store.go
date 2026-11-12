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

const guestTokenKeyPrefix = "auth:guest:"

// GuestTokenStore stores guest tokens in the cache with their TTL. Keys are
// derived from the token by the use case; tokens are never stored.
type GuestTokenStore struct {
	cache cache.Store
}

var _ auth.GuestTokenStore = (*GuestTokenStore)(nil)

func NewGuestTokenStore(c cache.Store) *GuestTokenStore {
	return &GuestTokenStore{cache: c}
}

type guestTokenRecord struct {
	EscrowID  string    `json:"escrow_id"`
	Scheme    string    `json:"scheme"`
	Address   string    `json:"address"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (s *GuestTokenStore) Save(ctx context.Context, key string, g domain.GuestToken, ttl time.Duration) error {
	b, err := json.Marshal(guestTokenRecord{
		EscrowID: g.EscrowID, Scheme: string(g.Wallet.Scheme), Address: g.Wallet.Address, ExpiresAt: g.ExpiresAt,
	})
	if err != nil {
		return fmt.Errorf("guest token store: encode: %w", err)
	}
	if err := s.cache.Set(ctx, guestTokenKeyPrefix+key, b, ttl); err != nil {
		return fmt.Errorf("guest token store: %w", err)
	}
	return nil
}

func (s *GuestTokenStore) Get(ctx context.Context, key string) (domain.GuestToken, error) {
	b, err := s.cache.Get(ctx, guestTokenKeyPrefix+key)
	if errors.Is(err, cache.ErrNotFound) {
		return domain.GuestToken{}, auth.ErrGuestTokenNotFound
	}
	if err != nil {
		return domain.GuestToken{}, fmt.Errorf("guest token store: %w", err)
	}
	var rec guestTokenRecord
	if err := json.Unmarshal(b, &rec); err != nil {
		return domain.GuestToken{}, fmt.Errorf("guest token store: decode: %w", err)
	}
	return domain.GuestToken{
		EscrowID:  rec.EscrowID,
		Wallet:    domain.Wallet{Scheme: domain.WalletScheme(rec.Scheme), Address: rec.Address},
		ExpiresAt: rec.ExpiresAt,
	}, nil
}
