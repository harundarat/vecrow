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

func TestSessionStore(t *testing.T) {
	mem := newMemoryCache()
	store := repository.NewSessionStore(mem)
	ctx := context.Background()
	sess := domain.Session{AccountID: "0193a1b2-0000-7000-8000-000000000001", ExpiresAt: time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)}

	if err := store.Save(ctx, "hash1", sess, 24*time.Hour); err != nil {
		t.Fatal(err)
	}
	if mem.ttls["auth:session:hash1"] != 24*time.Hour {
		t.Errorf("ttl = %v, want 24h", mem.ttls["auth:session:hash1"])
	}
	got, err := store.Get(ctx, "hash1")
	if err != nil || got.AccountID != sess.AccountID || !got.ExpiresAt.Equal(sess.ExpiresAt) {
		t.Fatalf("Get = %+v, %v", got, err)
	}
	if err := store.Delete(ctx, "hash1"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, "hash1"); !errors.Is(err, auth.ErrSessionNotFound) {
		t.Fatalf("Get after Delete: error = %v, want ErrSessionNotFound", err)
	}
}
