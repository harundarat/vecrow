package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/harundarat/vecrow/backend/internal/repository"
)

func TestJobLeases(t *testing.T) {
	ctx := context.Background()
	c := newMemoryCache()
	l := repository.NewJobLeases(c)

	token, ok, err := l.Acquire(ctx, "k", time.Minute)
	if err != nil || !ok || len(token) != 32 {
		t.Fatalf("Acquire = %q, %v, %v", token, ok, err)
	}
	if _, ok, err := l.Acquire(ctx, "k", time.Minute); err != nil || ok {
		t.Fatalf("second Acquire = %v, %v; want held", ok, err)
	}
	// Another holder's token does not free it.
	if err := l.Release(ctx, "k", "other"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := l.Acquire(ctx, "k", time.Minute); ok {
		t.Fatal("released with the wrong token")
	}
	if err := l.Release(ctx, "k", token); err != nil {
		t.Fatal(err)
	}
	token2, ok, err := l.Acquire(ctx, "k", time.Minute)
	if err != nil || !ok || token2 == token {
		t.Fatalf("Acquire after release = %q, %v, %v", token2, ok, err)
	}
	// An empty token frees it whoever holds it; releasing a free key is fine.
	if err := l.Release(ctx, "k", ""); err != nil {
		t.Fatal(err)
	}
	if err := l.Release(ctx, "k", token2); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := l.Acquire(ctx, "k", time.Minute); !ok {
		t.Fatal("not free after release")
	}
}
