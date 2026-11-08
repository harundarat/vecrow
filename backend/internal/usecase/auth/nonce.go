// Package auth implements wallet login (spec.md R-13).
package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"time"

	"github.com/harundarat/vecrow/backend/internal/domain"
)

// NonceStore keeps issued nonces until they are used or expire.
type NonceStore interface {
	Save(ctx context.Context, nonce domain.Nonce, ttl time.Duration) error
}

// NonceService issues login nonces.
type NonceService struct {
	store  NonceStore
	ttl    time.Duration
	random io.Reader
	now    func() time.Time
}

type Option func(*NonceService)

func WithRandom(r io.Reader) Option { return func(s *NonceService) { s.random = r } }

func WithClock(now func() time.Time) Option { return func(s *NonceService) { s.now = now } }

func NewNonceService(store NonceStore, ttl time.Duration, opts ...Option) *NonceService {
	s := &NonceService{store: store, ttl: ttl, random: rand.Reader, now: time.Now}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Issue creates a nonce for the wallet identified by scheme and address. The
// wallet signs it to log in.
func (s *NonceService) Issue(ctx context.Context, scheme, address string) (domain.Nonce, error) {
	sch, err := domain.ParseWalletScheme(scheme)
	if err != nil {
		return domain.Nonce{}, err
	}
	wallet, err := domain.NewWallet(sch, address)
	if err != nil {
		return domain.Nonce{}, err
	}

	buf := make([]byte, domain.NonceSize)
	if _, err := io.ReadFull(s.random, buf); err != nil {
		return domain.Nonce{}, fmt.Errorf("auth: generate nonce: %w", err)
	}
	nonce := domain.Nonce{
		Value:     hex.EncodeToString(buf),
		Wallet:    wallet,
		ExpiresAt: s.now().Add(s.ttl).UTC(),
	}
	if err := s.store.Save(ctx, nonce, s.ttl); err != nil {
		return domain.Nonce{}, fmt.Errorf("auth: save nonce: %w", err)
	}
	return nonce, nil
}
