package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/harundarat/vecrow/backend/internal/domain"
)

const sessionTokenSize = 32

// maxTokenLen bounds tokens accepted from clients; real ones are 43 chars.
const maxTokenLen = 128

// SessionService creates and checks bearer session tokens. Tokens are opaque
// random strings; only their SHA-256 is stored, so a leaked store does not
// leak usable tokens.
type SessionService struct {
	store SessionStore
	ttl   time.Duration
	rt    runtime
}

func NewSessionService(store SessionStore, ttl time.Duration, opts ...Option) *SessionService {
	return &SessionService{store: store, ttl: ttl, rt: newRuntime(opts)}
}

// Create starts a session for the account and returns its token.
func (s *SessionService) Create(ctx context.Context, accountID string) (string, domain.Session, error) {
	buf := make([]byte, sessionTokenSize)
	if _, err := io.ReadFull(s.rt.random, buf); err != nil {
		return "", domain.Session{}, fmt.Errorf("auth: generate session token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(buf)
	sess := domain.Session{AccountID: accountID, ExpiresAt: s.rt.now().Add(s.ttl).UTC()}
	if err := s.store.Save(ctx, sessionKey(token), sess, s.ttl); err != nil {
		return "", domain.Session{}, fmt.Errorf("auth: save session: %w", err)
	}
	return token, sess, nil
}

// Authenticate returns the session of token, or an Unauthorized error.
func (s *SessionService) Authenticate(ctx context.Context, token string) (domain.Session, error) {
	if token == "" || len(token) > maxTokenLen {
		return domain.Session{}, domain.Unauthorized("session is invalid or expired")
	}
	sess, err := s.store.Get(ctx, sessionKey(token))
	if errors.Is(err, ErrSessionNotFound) {
		return domain.Session{}, domain.Unauthorized("session is invalid or expired")
	}
	if err != nil {
		return domain.Session{}, fmt.Errorf("auth: get session: %w", err)
	}
	return sess, nil
}

// Revoke ends the session of token (logout). Unknown tokens are ignored.
func (s *SessionService) Revoke(ctx context.Context, token string) error {
	if err := s.store.Delete(ctx, sessionKey(token)); err != nil {
		return fmt.Errorf("auth: delete session: %w", err)
	}
	return nil
}

func sessionKey(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
