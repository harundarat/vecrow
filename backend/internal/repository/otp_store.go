package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/harundarat/vecrow/backend/internal/infrastructure/cache"
	"github.com/harundarat/vecrow/backend/internal/usecase/auth"
)

const (
	otpKeyPrefix  = "auth:otp:"
	rateKeyPrefix = "auth:rate:"
)

// OTPStore keeps one-time codes in the cache under
// auth:otp:<purpose>:<sha256(subject)>, with their attempt count in
// ...:attempts. Subjects (emails, account IDs) are hashed so the keys do not
// show email addresses; the codes themselves are stored only as an HMAC by
// the use case.
type OTPStore struct {
	cache cache.Store
}

var _ auth.OTPStore = (*OTPStore)(nil)

func NewOTPStore(c cache.Store) *OTPStore {
	return &OTPStore{cache: c}
}

type otpRecord struct {
	AccountID string    `json:"account_id"`
	Email     string    `json:"email"`
	Salt      []byte    `json:"salt"`
	MAC       []byte    `json:"mac"`
	ExpiresAt time.Time `json:"expires_at"`
}

func otpKey(purpose, subject string) string {
	sum := sha256.Sum256([]byte(subject))
	return otpKeyPrefix + purpose + ":" + hex.EncodeToString(sum[:])
}

func attemptsKey(purpose, subject string) string { return otpKey(purpose, subject) + ":attempts" }

func (s *OTPStore) Save(ctx context.Context, purpose, subject string, rec auth.OTPRecord, ttl time.Duration) error {
	b, err := json.Marshal(otpRecord(rec))
	if err != nil {
		return fmt.Errorf("otp store: encode: %w", err)
	}
	if err := s.cache.Delete(ctx, attemptsKey(purpose, subject)); err != nil {
		return fmt.Errorf("otp store: %w", err)
	}
	if err := s.cache.Set(ctx, otpKey(purpose, subject), b, ttl); err != nil {
		return fmt.Errorf("otp store: %w", err)
	}
	return nil
}

func (s *OTPStore) Get(ctx context.Context, purpose, subject string) (auth.OTPRecord, error) {
	return s.decode(s.cache.Get(ctx, otpKey(purpose, subject)))
}

func (s *OTPStore) Consume(ctx context.Context, purpose, subject string) (auth.OTPRecord, error) {
	rec, err := s.decode(s.cache.GetDel(ctx, otpKey(purpose, subject)))
	if err != nil {
		return auth.OTPRecord{}, err
	}
	if err := s.cache.Delete(ctx, attemptsKey(purpose, subject)); err != nil {
		return auth.OTPRecord{}, fmt.Errorf("otp store: %w", err)
	}
	return rec, nil
}

func (s *OTPStore) CountAttempt(ctx context.Context, purpose, subject string, ttl time.Duration) (int, error) {
	n, err := s.cache.Incr(ctx, attemptsKey(purpose, subject), ttl)
	if err != nil {
		return 0, fmt.Errorf("otp store: %w", err)
	}
	return int(n), nil
}

func (s *OTPStore) Delete(ctx context.Context, purpose, subject string) error {
	if err := s.cache.Delete(ctx, otpKey(purpose, subject)); err != nil {
		return fmt.Errorf("otp store: %w", err)
	}
	return nil
}

func (s *OTPStore) decode(b []byte, err error) (auth.OTPRecord, error) {
	if errors.Is(err, cache.ErrNotFound) {
		return auth.OTPRecord{}, auth.ErrOTPNotFound
	}
	if err != nil {
		return auth.OTPRecord{}, fmt.Errorf("otp store: %w", err)
	}
	var rec otpRecord
	if err := json.Unmarshal(b, &rec); err != nil {
		return auth.OTPRecord{}, fmt.Errorf("otp store: decode: %w", err)
	}
	return auth.OTPRecord(rec), nil
}

// RateLimiter counts events per key in fixed windows in the cache, under
// auth:rate:<sha256(key)>, so the limits hold across api instances.
type RateLimiter struct {
	cache cache.Store
}

var _ auth.RateLimiter = (*RateLimiter)(nil)

func NewRateLimiter(c cache.Store) *RateLimiter {
	return &RateLimiter{cache: c}
}

func (r *RateLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	sum := sha256.Sum256([]byte(key))
	n, err := r.cache.Incr(ctx, rateKeyPrefix+hex.EncodeToString(sum[:]), window)
	if err != nil {
		return false, fmt.Errorf("rate limiter: %w", err)
	}
	return n <= int64(limit), nil
}
