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

func TestGuestTokenService(t *testing.T) {
	store := guestMap{}
	now := fixedNow
	svc := auth.NewGuestTokenService(store, 24*time.Hour, auth.WithClock(func() time.Time { return now }))
	ctx := context.Background()
	w := domain.Wallet{Scheme: domain.SchemeEIP191, Address: "0x7e5f4552091a69125d5dfcb7b8c2659029395bdf"}

	token, exp, err := svc.Issue(ctx, "escrow-1", w)
	if err != nil {
		t.Fatal(err)
	}
	if len(token) != 43 || !exp.Equal(fixedNow.Add(24*time.Hour)) {
		t.Fatalf("token %q expires %v", token, exp)
	}
	for key := range store {
		if strings.Contains(key, token) {
			t.Fatal("the raw token must not be stored")
		}
	}
	g, err := svc.Authenticate(ctx, token)
	if err != nil || g != (domain.GuestToken{EscrowID: "escrow-1", Wallet: w, ExpiresAt: exp}) {
		t.Fatalf("Authenticate = %+v, %v", g, err)
	}
	for name, bad := range map[string]string{"empty": "", "unknown": token + "x", "too long": strings.Repeat("a", 200)} {
		if _, err := svc.Authenticate(ctx, bad); !errors.Is(err, domain.ErrUnauthorized) {
			t.Errorf("%s token: %v", name, err)
		}
	}
	now = exp
	if _, err := svc.Authenticate(ctx, token); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("expired token: %v", err)
	}
}

func TestWalletProver(t *testing.T) {
	ctx := context.Background()
	nonces := nonceMap{}
	verifier := &fakeVerifier{}
	w := domain.Wallet{Scheme: domain.SchemeNEP413, Address: "alice.near"}
	value := strings.Repeat("ab", 32)
	nonces[value] = domain.Nonce{Value: value, Wallet: w}
	p := auth.NewWalletProver(nonces, verifier)

	got, err := p.Prove(ctx, auth.Proof{Nonce: value, Signature: "sig"})
	if err != nil || got != w {
		t.Fatalf("Prove = %+v, %v", got, err)
	}
	if _, err := p.Prove(ctx, auth.Proof{Nonce: value, Signature: "sig"}); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("reused nonce: %v", err)
	}
	nonces[value] = domain.Nonce{Value: value, Wallet: w}
	verifier.err = domain.Unauthorized("bad signature")
	if _, err := p.Prove(ctx, auth.Proof{Nonce: value, Signature: "sig"}); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("bad signature: %v", err)
	}
}
