package auth_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/harundarat/vecrow/backend/internal/domain"
	"github.com/harundarat/vecrow/backend/internal/usecase/auth"
)

func TestSessionService(t *testing.T) {
	store := sessionMap{}
	svc := auth.NewSessionService(store, 24*time.Hour, auth.WithClock(func() time.Time { return fixedNow }))
	ctx := context.Background()

	token, sess, err := svc.Create(ctx, "acct-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(token) != 43 || strings.ContainsAny(token, "+/=") {
		t.Errorf("token %q: want 43 base64url characters", token)
	}
	if want := (domain.Session{AccountID: "acct-1", ExpiresAt: fixedNow.Add(24 * time.Hour)}); sess != want {
		t.Errorf("session = %+v, want %+v", sess, want)
	}
	for key := range store {
		if strings.Contains(key, token) {
			t.Error("the raw token must not be stored")
		}
	}

	got, err := svc.Authenticate(ctx, token)
	if err != nil || got != sess {
		t.Fatalf("Authenticate = %+v, %v", got, err)
	}

	for name, bad := range map[string]string{
		"empty":    "",
		"unknown":  "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"too long": strings.Repeat("a", 200),
	} {
		if _, err := svc.Authenticate(ctx, bad); !errors.Is(err, domain.ErrUnauthorized) {
			t.Errorf("%s token: error = %v, want ErrUnauthorized", name, err)
		}
	}

	if err := svc.Revoke(ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Authenticate(ctx, token); !errors.Is(err, domain.ErrUnauthorized) {
		t.Errorf("revoked token: error = %v, want ErrUnauthorized", err)
	}
}
