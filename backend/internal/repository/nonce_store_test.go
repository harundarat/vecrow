package repository_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/harundarat/vecrow/backend/internal/domain"
	"github.com/harundarat/vecrow/backend/internal/infrastructure/cache"
	"github.com/harundarat/vecrow/backend/internal/repository"
	"github.com/harundarat/vecrow/backend/internal/usecase/auth"
)

// memoryCache is an in-memory cache.Store for tests.
type memoryCache struct {
	values map[string][]byte
	ttls   map[string]time.Duration
	err    error
}

var _ cache.Store = (*memoryCache)(nil)

func newMemoryCache() *memoryCache {
	return &memoryCache{values: map[string][]byte{}, ttls: map[string]time.Duration{}}
}

func (m *memoryCache) Get(_ context.Context, key string) ([]byte, error) {
	v, ok := m.values[key]
	if !ok {
		return nil, cache.ErrNotFound
	}
	return v, nil
}

func (m *memoryCache) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	m.values[key], m.ttls[key] = value, ttl
	return nil
}

func (m *memoryCache) SetNX(_ context.Context, key string, value []byte, ttl time.Duration) (bool, error) {
	if m.err != nil {
		return false, m.err
	}
	if _, ok := m.values[key]; ok {
		return false, nil
	}
	m.values[key], m.ttls[key] = value, ttl
	return true, nil
}

func (m *memoryCache) GetDel(ctx context.Context, key string) ([]byte, error) {
	v, err := m.Get(ctx, key)
	delete(m.values, key)
	return v, err
}

func (m *memoryCache) Delete(_ context.Context, keys ...string) error {
	for _, k := range keys {
		delete(m.values, k)
	}
	return nil
}

func testNonce() domain.Nonce {
	return domain.Nonce{
		Value:  "deadbeef",
		Wallet: domain.Wallet{Scheme: domain.SchemeNEP413, Address: "alice.near"},
	}
}

func TestNonceStore_Save(t *testing.T) {
	mem := newMemoryCache()
	store := repository.NewNonceStore(mem)

	if err := store.Save(context.Background(), testNonce(), 5*time.Minute); err != nil {
		t.Fatalf("Save: %v", err)
	}

	const key = "auth:nonce:deadbeef"
	var rec map[string]string
	if err := json.Unmarshal(mem.values[key], &rec); err != nil {
		t.Fatalf("stored value is not JSON: %v", err)
	}
	if rec["scheme"] != "nep413" || rec["address"] != "alice.near" {
		t.Errorf("stored record = %v", rec)
	}
	if mem.ttls[key] != 5*time.Minute {
		t.Errorf("ttl = %v, want 5m", mem.ttls[key])
	}
}

func TestNonceStore_Save_Duplicate(t *testing.T) {
	store := repository.NewNonceStore(newMemoryCache())
	if err := store.Save(context.Background(), testNonce(), time.Minute); err != nil {
		t.Fatal(err)
	}
	err := store.Save(context.Background(), testNonce(), time.Minute)
	if !errors.Is(err, repository.ErrNonceExists) {
		t.Fatalf("error = %v, want ErrNonceExists", err)
	}
}

func TestNonceStore_Save_CacheError(t *testing.T) {
	mem := newMemoryCache()
	mem.err = errors.New("cache down")
	err := repository.NewNonceStore(mem).Save(context.Background(), testNonce(), time.Minute)
	if !errors.Is(err, mem.err) {
		t.Fatalf("error = %v, want wrapped cache error", err)
	}
}

func TestNonceStore_Consume(t *testing.T) {
	mem := newMemoryCache()
	store := repository.NewNonceStore(mem)
	n := testNonce()
	n.ExpiresAt = time.Date(2026, 11, 8, 12, 5, 0, 0, time.UTC)
	n.Message = "localhost:5173 wants you to sign in with your wallet."
	if err := store.Save(context.Background(), n, time.Minute); err != nil {
		t.Fatal(err)
	}

	got, err := store.Consume(context.Background(), n.Value)
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if got.Value != n.Value || got.Wallet != n.Wallet || got.Message != n.Message || !got.ExpiresAt.Equal(n.ExpiresAt) {
		t.Errorf("Consume = %+v, want %+v", got, n)
	}
	if _, err := store.Consume(context.Background(), n.Value); !errors.Is(err, auth.ErrNonceNotFound) {
		t.Fatalf("second Consume error = %v, want ErrNonceNotFound", err)
	}
}
