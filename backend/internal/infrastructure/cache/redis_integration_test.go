//go:build integration

package cache_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/harundarat/vecrow/backend/internal/infrastructure/cache"
)

func openRedis(t *testing.T) *cache.Redis {
	t.Helper()
	url := os.Getenv("CACHE_URL")
	if url == "" {
		t.Skip("CACHE_URL not set")
	}
	r, err := cache.OpenRedis(context.Background(), cache.Config{URL: url, StartupTimeout: 5 * time.Second}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("OpenRedis: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

func TestRedis_Store(t *testing.T) {
	ctx := context.Background()
	r := openRedis(t)
	key := "it:" + uuid.NewString()
	t.Cleanup(func() { _ = r.Delete(ctx, key) })

	if _, err := r.Get(ctx, key); !errors.Is(err, cache.ErrNotFound) {
		t.Fatalf("Get missing key error = %v, want ErrNotFound", err)
	}

	ok, err := r.SetNX(ctx, key, []byte("v1"), time.Minute)
	if err != nil || !ok {
		t.Fatalf("first SetNX = %v, %v; want true", ok, err)
	}
	ok, err = r.SetNX(ctx, key, []byte("v2"), time.Minute)
	if err != nil || ok {
		t.Fatalf("second SetNX = %v, %v; want false", ok, err)
	}

	if got, err := r.Get(ctx, key); err != nil || string(got) != "v1" {
		t.Fatalf("Get = %q, %v; want v1", got, err)
	}
	if got, err := r.GetDel(ctx, key); err != nil || string(got) != "v1" {
		t.Fatalf("GetDel = %q, %v; want v1", got, err)
	}
	if _, err := r.GetDel(ctx, key); !errors.Is(err, cache.ErrNotFound) {
		t.Fatalf("GetDel after delete error = %v, want ErrNotFound", err)
	}

	if err := r.Set(ctx, key, []byte("short"), 100*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	time.Sleep(250 * time.Millisecond)
	if _, err := r.Get(ctx, key); !errors.Is(err, cache.ErrNotFound) {
		t.Fatalf("expired key error = %v, want ErrNotFound", err)
	}

	if err := r.Check(ctx); err != nil {
		t.Fatalf("Check: %v", err)
	}
}
