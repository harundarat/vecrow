package auth_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/harundarat/vecrow/backend/internal/domain"
	"github.com/harundarat/vecrow/backend/internal/usecase/auth"
)

type fakeStore struct {
	saved []domain.Nonce
	ttls  []time.Duration
	err   error
}

func (f *fakeStore) Save(_ context.Context, n domain.Nonce, ttl time.Duration) error {
	if f.err != nil {
		return f.err
	}
	f.saved = append(f.saved, n)
	f.ttls = append(f.ttls, ttl)
	return nil
}

func (f *fakeStore) Consume(context.Context, string) (domain.Nonce, error) {
	return domain.Nonce{}, auth.ErrNonceNotFound
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("entropy exhausted") }

var fixedNow = time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

func newService(store auth.NonceStore, opts ...auth.Option) *auth.NonceService {
	defaults := []auth.Option{
		auth.WithRandom(bytes.NewReader(bytes.Repeat([]byte{0xab}, domain.NonceSize))),
		auth.WithClock(func() time.Time { return fixedNow }),
	}
	return auth.NewNonceService(store, 5*time.Minute, "localhost:5173", append(defaults, opts...)...)
}

func TestNonceService_Issue(t *testing.T) {
	store := &fakeStore{}
	svc := newService(store)

	got, err := svc.Issue(context.Background(), "eip191", "0xAbCdEf0123456789abcdef0123456789ABCDEF01")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	if want := strings.Repeat("ab", domain.NonceSize); got.Value != want {
		t.Errorf("nonce = %q, want %q", got.Value, want)
	}
	if want := fixedNow.Add(5 * time.Minute); !got.ExpiresAt.Equal(want) {
		t.Errorf("expires_at = %v, want %v", got.ExpiresAt, want)
	}
	if got.Wallet.Address != "0xabcdef0123456789abcdef0123456789abcdef01" {
		t.Errorf("address not normalised: %q", got.Wallet.Address)
	}
	if got.Message != domain.LoginMessage("localhost:5173", got) || !strings.Contains(got.Message, got.Wallet.Address) {
		t.Errorf("message = %q, want the login message for the nonce", got.Message)
	}
	if svc.Recipient() != "localhost:5173" {
		t.Errorf("recipient = %q", svc.Recipient())
	}
	if len(store.saved) != 1 || store.saved[0] != got || store.ttls[0] != 5*time.Minute {
		t.Errorf("store got %+v ttl %v, want the issued nonce with 5m ttl", store.saved, store.ttls)
	}
}

func TestNonceService_Issue_InvalidInput(t *testing.T) {
	tests := map[string][2]string{
		"unknown scheme":  {"rsa", "alice.near"},
		"invalid address": {"nep413", "NOT A NEAR ACCOUNT"},
	}
	for name, in := range tests {
		t.Run(name, func(t *testing.T) {
			store := &fakeStore{}
			_, err := newService(store).Issue(context.Background(), in[0], in[1])
			if !errors.Is(err, domain.ErrInvalidArgument) {
				t.Fatalf("error = %v, want ErrInvalidArgument", err)
			}
			if len(store.saved) != 0 {
				t.Fatal("store must not be called for invalid input")
			}
		})
	}
}

func TestNonceService_Issue_InfrastructureErrors(t *testing.T) {
	t.Run("store fails", func(t *testing.T) {
		storeErr := errors.New("connection refused")
		_, err := newService(&fakeStore{err: storeErr}).Issue(context.Background(), "nep413", "alice.near")
		if !errors.Is(err, storeErr) {
			t.Fatalf("error = %v, want wrapped store error", err)
		}
		var de *domain.Error
		if errors.As(err, &de) {
			t.Fatal("infrastructure errors must not be reported as domain errors")
		}
	})
	t.Run("random source fails", func(t *testing.T) {
		_, err := newService(&fakeStore{}, auth.WithRandom(failingReader{})).Issue(context.Background(), "nep413", "alice.near")
		if err == nil || !strings.Contains(err.Error(), "generate nonce") {
			t.Fatalf("error = %v, want generate nonce error", err)
		}
	})
}
