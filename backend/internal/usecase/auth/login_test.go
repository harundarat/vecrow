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

var testWallet = domain.Wallet{Scheme: domain.SchemeEIP191, Address: "0xf39fd6e51aad88f6f4ce6ab8827279cfffb92266"}

type loginFixture struct {
	nonces   nonceMap
	verifier *fakeVerifier
	accounts *fakeAccounts
	sessions sessionMap
	svc      *auth.LoginService
}

// newLoginFixture builds a LoginService whose username generator reads
// random (0xab bytes give "user_11111111").
func newLoginFixture(random []byte) *loginFixture {
	f := &loginFixture{
		nonces:   nonceMap{},
		verifier: &fakeVerifier{},
		accounts: newFakeAccounts(),
		sessions: sessionMap{},
	}
	sessions := auth.NewSessionService(f.sessions, time.Hour, auth.WithClock(func() time.Time { return fixedNow }))
	if random == nil {
		random = bytes.Repeat([]byte{0xab}, 64)
	}
	f.svc = auth.NewLoginService(f.nonces, f.verifier, f.accounts, fakeTx{f.accounts}, sessions,
		auth.WithRandom(bytes.NewReader(random)))
	return f
}

// issue stores a nonce for w and returns a proof for it.
func (f *loginFixture) issue(w domain.Wallet, value string) auth.Proof {
	f.nonces[value] = domain.Nonce{Value: value, Wallet: w, Message: "sign me"}
	return auth.Proof{Nonce: value, Signature: "0xsig"}
}

func nonceValue(b byte) string { return strings.Repeat(string("0123456789abcdef"[b%16]), 64) }

func TestLogin_FirstLoginCreatesAccount(t *testing.T) {
	f := newLoginFixture(nil)

	res, err := f.svc.Login(context.Background(), f.issue(testWallet, nonceValue(1)))
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if !res.NewAccount || res.Account.Username != "user_11111111" {
		t.Errorf("result = %+v, want a new account named user_11111111", res)
	}
	if len(res.Account.Wallets) != 1 || res.Account.Wallets[0].Wallet != testWallet {
		t.Errorf("wallets = %+v, want the login wallet", res.Account.Wallets)
	}
	if res.Token == "" || !res.Session.ExpiresAt.Equal(fixedNow.Add(time.Hour)) || res.Session.AccountID != res.Account.ID {
		t.Errorf("session = %+v token %q", res.Session, res.Token)
	}
	if _, stored := f.sessions[res.Token]; stored || len(f.sessions) != 1 {
		t.Errorf("sessions = %v: want one session not keyed by the raw token", f.sessions)
	}
}

func TestLogin_SameWalletOpensSameAccount(t *testing.T) {
	f := newLoginFixture(nil)
	first, err := f.svc.Login(context.Background(), f.issue(testWallet, nonceValue(1)))
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.svc.Login(context.Background(), f.issue(testWallet, nonceValue(2)))
	if err != nil {
		t.Fatal(err)
	}
	if second.NewAccount || second.Account.ID != first.Account.ID {
		t.Errorf("second login = %+v, want account %s", second, first.Account.ID)
	}
	if second.Token == first.Token {
		t.Error("each login must get its own session token")
	}
	if len(f.accounts.accounts) != 1 {
		t.Errorf("accounts = %d, want 1", len(f.accounts.accounts))
	}
}

func TestLogin_RejectsReusedNonce(t *testing.T) {
	f := newLoginFixture(nil)
	proof := f.issue(testWallet, nonceValue(1))
	if _, err := f.svc.Login(context.Background(), proof); err != nil {
		t.Fatal(err)
	}
	_, err := f.svc.Login(context.Background(), proof)
	if !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("error = %v, want ErrUnauthorized", err)
	}
}

func TestLogin_InvalidSignatureBurnsNonce(t *testing.T) {
	f := newLoginFixture(nil)
	f.verifier.err = domain.Unauthorized("signature is invalid")
	proof := f.issue(testWallet, nonceValue(1))

	if _, err := f.svc.Login(context.Background(), proof); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("error = %v, want ErrUnauthorized", err)
	}
	f.verifier.err = nil
	_, err := f.svc.Login(context.Background(), proof)
	if !errors.Is(err, domain.ErrUnauthorized) || !strings.Contains(err.Error(), "nonce") {
		t.Fatalf("retry error = %v, want the nonce to be used up", err)
	}
	if len(f.accounts.accounts) != 0 || len(f.sessions) != 0 {
		t.Error("a failed login must not create an account or session")
	}
}

func TestLogin_MalformedNonceIsRejectedBeforeLookup(t *testing.T) {
	f := newLoginFixture(nil)
	_, err := f.svc.Login(context.Background(), auth.Proof{Nonce: "not-hex", Signature: "x"})
	if !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("error = %v, want ErrInvalidArgument", err)
	}
	if f.verifier.calls != 0 {
		t.Error("verifier must not be called")
	}
}

func TestLogin_RetriesTakenUsername(t *testing.T) {
	// First generated name is user_11111111 (taken), the second user_aaaaaaaa.
	random := append(bytes.Repeat([]byte{0xab}, 8), bytes.Repeat([]byte{0x00}, 8)...)
	f := newLoginFixture(random)
	f.accounts.add("user_11111111")

	res, err := f.svc.Login(context.Background(), f.issue(testWallet, nonceValue(1)))
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if res.Account.Username != "user_aaaaaaaa" {
		t.Errorf("username = %q, want user_aaaaaaaa", res.Account.Username)
	}
}

func TestLogin_ConcurrentFirstLoginJoinsExistingAccount(t *testing.T) {
	f := newLoginFixture(nil)
	owner := f.accounts.add("alice", testWallet)
	f.accounts.hideOwnerOnce = true // the other login commits after our lookup

	res, err := f.svc.Login(context.Background(), f.issue(testWallet, nonceValue(1)))
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if res.NewAccount || res.Account.ID != owner {
		t.Errorf("result = %+v, want existing account %s", res, owner)
	}
	if len(f.accounts.accounts) != 1 {
		t.Errorf("accounts = %d, want the losing account rolled back", len(f.accounts.accounts))
	}
}

func TestLogin_InfrastructureErrorIsNotADomainError(t *testing.T) {
	f := newLoginFixture(nil)
	f.accounts.err = errors.New("connection refused")
	_, err := f.svc.Login(context.Background(), f.issue(testWallet, nonceValue(1)))
	var de *domain.Error
	if err == nil || errors.As(err, &de) || !errors.Is(err, f.accounts.err) {
		t.Fatalf("error = %v, want the wrapped infrastructure error", err)
	}
}

func TestLogout_RevokesSession(t *testing.T) {
	f := newLoginFixture(nil)
	res, err := f.svc.Login(context.Background(), f.issue(testWallet, nonceValue(1)))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Logout(context.Background(), res.Token); err != nil {
		t.Fatal(err)
	}
	if len(f.sessions) != 0 {
		t.Errorf("sessions = %v, want none after logout", f.sessions)
	}
}
