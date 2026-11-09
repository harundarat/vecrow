// Package auth implements wallet login, sessions and account management
// (spec.md R-13, R-14).
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

// runtime holds the sources of randomness and time, replaceable in tests.
type runtime struct {
	random io.Reader
	now    func() time.Time
}

// Option configures a service of this package.
type Option func(*runtime)

func WithRandom(r io.Reader) Option { return func(rt *runtime) { rt.random = r } }

func WithClock(now func() time.Time) Option { return func(rt *runtime) { rt.now = now } }

func newRuntime(opts []Option) runtime {
	rt := runtime{random: rand.Reader, now: time.Now}
	for _, opt := range opts {
		opt(&rt)
	}
	return rt
}

// NonceService issues login nonces.
type NonceService struct {
	store     NonceStore
	ttl       time.Duration
	appDomain string
	rt        runtime
}

// NewNonceService returns a service whose messages name appDomain.
func NewNonceService(store NonceStore, ttl time.Duration, appDomain string, opts ...Option) *NonceService {
	return &NonceService{store: store, ttl: ttl, appDomain: appDomain, rt: newRuntime(opts)}
}

// Recipient is the NEP-413 recipient NEAR wallets must sign for.
func (s *NonceService) Recipient() string { return s.appDomain }

// Issue creates a nonce for the wallet identified by scheme and address. The
// wallet signs its Message to log in or to be linked to an account.
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
	if _, err := io.ReadFull(s.rt.random, buf); err != nil {
		return domain.Nonce{}, fmt.Errorf("auth: generate nonce: %w", err)
	}
	nonce := domain.Nonce{
		Value:     hex.EncodeToString(buf),
		Wallet:    wallet,
		ExpiresAt: s.rt.now().Add(s.ttl).UTC(),
	}
	nonce.Message = domain.LoginMessage(s.appDomain, nonce)
	if err := s.store.Save(ctx, nonce, s.ttl); err != nil {
		return domain.Nonce{}, fmt.Errorf("auth: save nonce: %w", err)
	}
	return nonce, nil
}
