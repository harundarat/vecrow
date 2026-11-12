package auth

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/harundarat/vecrow/backend/internal/domain"
)

// ErrGuestTokenNotFound is returned by GuestTokenStore for unknown or
// expired tokens.
var ErrGuestTokenNotFound = errors.New("auth: guest token not found")

// GuestTokenStore keeps guest tokens under a key derived from the token; the
// token itself is never stored.
type GuestTokenStore interface {
	Save(ctx context.Context, key string, g domain.GuestToken, ttl time.Duration) error
	// Get returns ErrGuestTokenNotFound for unknown or expired tokens.
	Get(ctx context.Context, key string) (domain.GuestToken, error)
}

// GuestTokenService issues and checks guest tokens (R-21): opaque tokens
// that let the wallet that submitted to an escrow view it, and the approver
// request quotes, without an account. They are never proof of a right in the
// contract, which checks signatures.
type GuestTokenService struct {
	store GuestTokenStore
	ttl   time.Duration
	rt    runtime
}

func NewGuestTokenService(store GuestTokenStore, ttl time.Duration, opts ...Option) *GuestTokenService {
	return &GuestTokenService{store: store, ttl: ttl, rt: newRuntime(opts)}
}

// Issue returns a new token for wallet w on escrow escrowID.
func (s *GuestTokenService) Issue(ctx context.Context, escrowID string, w domain.Wallet) (string, time.Time, error) {
	buf := make([]byte, sessionTokenSize)
	if _, err := io.ReadFull(s.rt.random, buf); err != nil {
		return "", time.Time{}, fmt.Errorf("auth: generate guest token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(buf)
	g := domain.GuestToken{EscrowID: escrowID, Wallet: w, ExpiresAt: s.rt.now().Add(s.ttl).UTC()}
	if err := s.store.Save(ctx, sessionKey(token), g, s.ttl); err != nil {
		return "", time.Time{}, fmt.Errorf("auth: save guest token: %w", err)
	}
	s.rt.log.InfoContext(ctx, "guest token issued", "escrow_id", escrowID, "scheme", string(w.Scheme))
	return token, g.ExpiresAt, nil
}

// Authenticate returns what token grants, or an Unauthorized error for an
// unknown or expired token.
func (s *GuestTokenService) Authenticate(ctx context.Context, token string) (domain.GuestToken, error) {
	invalid := domain.Unauthorized("guest token is invalid or expired; renew it with a signature")
	if token == "" || len(token) > maxTokenLen {
		return domain.GuestToken{}, invalid
	}
	g, err := s.store.Get(ctx, sessionKey(token))
	if errors.Is(err, ErrGuestTokenNotFound) {
		return domain.GuestToken{}, invalid
	}
	if err != nil {
		return domain.GuestToken{}, fmt.Errorf("auth: get guest token: %w", err)
	}
	if !s.rt.now().Before(g.ExpiresAt) {
		return domain.GuestToken{}, invalid
	}
	return g, nil
}

// WalletProver proves control of a wallet with a signature over the message
// of a nonce from POST /auth/nonce, without logging in.
type WalletProver struct {
	nonces   NonceStore
	verifier SignatureVerifier
}

func NewWalletProver(nonces NonceStore, verifier SignatureVerifier) *WalletProver {
	return &WalletProver{nonces: nonces, verifier: verifier}
}

// Prove consumes the nonce of p and returns the wallet that signed it.
func (p *WalletProver) Prove(ctx context.Context, proof Proof) (domain.Wallet, error) {
	return proveWallet(ctx, p.nonces, p.verifier, proof)
}
