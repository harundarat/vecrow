package repository_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/harundarat/vecrow/backend/internal/repository"
	"github.com/harundarat/vecrow/backend/internal/usecase/auth"
)

func TestOTPStore(t *testing.T) {
	ctx := context.Background()
	mem := newMemoryCache()
	store := repository.NewOTPStore(mem)
	rec := auth.OTPRecord{
		AccountID: "acct-1", Email: "alice@example.com",
		Salt: []byte("salt"), MAC: []byte("mac"), ExpiresAt: time.Date(2026, 11, 8, 12, 10, 0, 0, time.UTC),
	}

	if _, err := store.Get(ctx, "login", "alice@example.com"); !errors.Is(err, auth.ErrOTPNotFound) {
		t.Fatalf("Get before Save: error = %v", err)
	}
	if err := store.Save(ctx, "login", "alice@example.com", rec, 10*time.Minute); err != nil {
		t.Fatal(err)
	}
	// Keys hash the subject so they do not show email addresses.
	for key, ttl := range mem.ttls {
		if strings.Contains(key, "alice") || !strings.HasPrefix(key, "auth:otp:login:") {
			t.Errorf("key %q", key)
		}
		if ttl != 10*time.Minute {
			t.Errorf("ttl of %q = %v", key, ttl)
		}
	}
	got, err := store.Get(ctx, "login", "alice@example.com")
	if err != nil || got.AccountID != "acct-1" || string(got.MAC) != "mac" || !got.ExpiresAt.Equal(rec.ExpiresAt) {
		t.Fatalf("Get = %+v, %v", got, err)
	}
	// Purposes do not share codes.
	if _, err := store.Get(ctx, "email", "alice@example.com"); !errors.Is(err, auth.ErrOTPNotFound) {
		t.Fatalf("Get of another purpose: error = %v", err)
	}

	for want := 1; want <= 3; want++ {
		if n, err := store.CountAttempt(ctx, "login", "alice@example.com", 10*time.Minute); err != nil || n != want {
			t.Fatalf("CountAttempt = %d, %v; want %d", n, err, want)
		}
	}
	// Delete keeps the attempts, so deleting a code does not reset them.
	if err := store.Delete(ctx, "login", "alice@example.com"); err != nil {
		t.Fatal(err)
	}
	if n, _ := store.CountAttempt(ctx, "login", "alice@example.com", 10*time.Minute); n != 4 {
		t.Fatalf("attempts after Delete = %d, want 4", n)
	}
	// Save resets them.
	if err := store.Save(ctx, "login", "alice@example.com", rec, 10*time.Minute); err != nil {
		t.Fatal(err)
	}
	if n, _ := store.CountAttempt(ctx, "login", "alice@example.com", 10*time.Minute); n != 1 {
		t.Fatalf("attempts after Save = %d, want 1", n)
	}
	// Consume returns the code once and resets the attempts.
	if got, err := store.Consume(ctx, "login", "alice@example.com"); err != nil || got.AccountID != "acct-1" {
		t.Fatalf("Consume = %+v, %v", got, err)
	}
	if _, err := store.Consume(ctx, "login", "alice@example.com"); !errors.Is(err, auth.ErrOTPNotFound) {
		t.Fatalf("second Consume: error = %v", err)
	}
	if n, _ := store.CountAttempt(ctx, "login", "alice@example.com", 10*time.Minute); n != 1 {
		t.Fatalf("attempts after Consume = %d, want 1", n)
	}
}

func TestRateLimiter(t *testing.T) {
	ctx := context.Background()
	mem := newMemoryCache()
	limiter := repository.NewRateLimiter(mem)
	for i := range 3 {
		if ok, err := limiter.Allow(ctx, "otp:alice@example.com", 3, time.Hour); err != nil || !ok {
			t.Fatalf("event %d: %v, %v", i+1, ok, err)
		}
	}
	if ok, err := limiter.Allow(ctx, "otp:alice@example.com", 3, time.Hour); err != nil || ok {
		t.Fatalf("4th event: %v, %v", ok, err)
	}
	if ok, _ := limiter.Allow(ctx, "otp:bob@example.com", 3, time.Hour); !ok {
		t.Fatal("the limit is shared between keys")
	}
	for key, ttl := range mem.ttls {
		if strings.Contains(key, "example.com") || !strings.HasPrefix(key, "auth:rate:") || ttl != time.Hour {
			t.Errorf("key %q ttl %v", key, ttl)
		}
	}
	mem.err = errors.New("redis down")
	if _, err := limiter.Allow(ctx, "otp:carol@example.com", 3, time.Hour); err == nil {
		t.Fatal("Allow ignored a cache error")
	}
}
