package repository_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/harundarat/vecrow/backend/internal/domain"
	"github.com/harundarat/vecrow/backend/internal/repository"
	"github.com/harundarat/vecrow/backend/internal/usecase/auth"
)

func TestGuestTokenStore(t *testing.T) {
	mem := newMemoryCache()
	store := repository.NewGuestTokenStore(mem)
	ctx := context.Background()
	g := domain.GuestToken{
		EscrowID:  "6f1c2a5e-8d3b-4c1e-9f00-1234567890ab",
		Wallet:    domain.Wallet{Scheme: domain.SchemeNEP413, Address: "alice.near"},
		ExpiresAt: time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC),
	}
	if err := store.Save(ctx, "hash1", g, 24*time.Hour); err != nil {
		t.Fatal(err)
	}
	if mem.ttls["auth:guest:hash1"] != 24*time.Hour {
		t.Errorf("ttl = %v, want 24h", mem.ttls["auth:guest:hash1"])
	}
	got, err := store.Get(ctx, "hash1")
	if err != nil || got.EscrowID != g.EscrowID || got.Wallet != g.Wallet || !got.ExpiresAt.Equal(g.ExpiresAt) {
		t.Fatalf("Get = %+v, %v", got, err)
	}
	if _, err := store.Get(ctx, "other"); !errors.Is(err, auth.ErrGuestTokenNotFound) {
		t.Fatalf("Get(unknown) = %v", err)
	}
}
