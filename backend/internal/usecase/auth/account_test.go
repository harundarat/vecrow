package auth_test

import (
	"context"
	"errors"
	"testing"

	"github.com/harundarat/vecrow/backend/internal/domain"
	"github.com/harundarat/vecrow/backend/internal/usecase/auth"
)

var solanaWallet = domain.Wallet{Scheme: domain.SchemeEd25519, Address: "9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM"}

type accountFixture struct {
	nonces   nonceMap
	verifier *fakeVerifier
	accounts *fakeAccounts
	svc      *auth.AccountService
}

func newAccountFixture() *accountFixture {
	f := &accountFixture{nonces: nonceMap{}, verifier: &fakeVerifier{}, accounts: newFakeAccounts()}
	f.svc = auth.NewAccountService(f.nonces, f.verifier, f.accounts, fakeTx{f.accounts})
	return f
}

func (f *accountFixture) issue(w domain.Wallet) auth.Proof {
	value := nonceValue(byte(len(f.nonces) + 1))
	f.nonces[value] = domain.Nonce{Value: value, Wallet: w, Message: "sign me"}
	return auth.Proof{Nonce: value, Signature: "sig"}
}

func TestAccount_Get(t *testing.T) {
	f := newAccountFixture()
	id := f.accounts.add("alice", testWallet)
	got, err := f.svc.Get(context.Background(), id)
	if err != nil || got.Username != "alice" || len(got.Wallets) != 1 {
		t.Fatalf("Get = %+v, %v", got, err)
	}
	if _, err := f.svc.Get(context.Background(), "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("unknown account: error = %v, want ErrNotFound", err)
	}
}

func TestAccount_ChangeUsername(t *testing.T) {
	f := newAccountFixture()
	id := f.accounts.add("user_11111111", testWallet)
	f.accounts.add("bob")

	got, err := f.svc.ChangeUsername(context.Background(), id, " Alice ")
	if err != nil || got.Username != "alice" {
		t.Fatalf("ChangeUsername = %+v, %v; want alice", got, err)
	}
	if _, err := f.svc.ChangeUsername(context.Background(), id, "BOB"); !errors.Is(err, domain.ErrConflict) {
		t.Errorf("taken username: error = %v, want ErrConflict", err)
	}
	if _, err := f.svc.ChangeUsername(context.Background(), id, "a!"); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Errorf("invalid username: error = %v, want ErrInvalidArgument", err)
	}
	if _, err := f.svc.ChangeUsername(context.Background(), id, "alice"); err != nil {
		t.Errorf("keeping the own username: error = %v", err)
	}
}

func TestAccount_LinkWallet(t *testing.T) {
	f := newAccountFixture()
	id := f.accounts.add("alice", testWallet)

	got, linked, err := f.svc.LinkWallet(context.Background(), id, f.issue(solanaWallet))
	if err != nil || !linked || len(got.Wallets) != 2 {
		t.Fatalf("LinkWallet = %+v, %v, %v; want 2 wallets", got, linked, err)
	}

	t.Run("already linked to this account is idempotent", func(t *testing.T) {
		got, linked, err := f.svc.LinkWallet(context.Background(), id, f.issue(solanaWallet))
		if err != nil || linked || len(got.Wallets) != 2 {
			t.Fatalf("LinkWallet = %+v, %v, %v", got, linked, err)
		}
	})
	t.Run("owned by another account", func(t *testing.T) {
		other := domain.Wallet{Scheme: domain.SchemeNEP413, Address: "bob.near"}
		f.accounts.add("bob", other)
		_, _, err := f.svc.LinkWallet(context.Background(), id, f.issue(other))
		if !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("error = %v, want ErrConflict", err)
		}
	})
	t.Run("owner unlinks it during the lookup", func(t *testing.T) {
		other := domain.Wallet{Scheme: domain.SchemeNEP413, Address: "dave.near"}
		f.accounts.add("dave", other)
		f.accounts.hideOwnerOnce = true
		_, _, err := f.svc.LinkWallet(context.Background(), id, f.issue(other))
		if !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("error = %v, want ErrConflict after the retry", err)
		}
	})
	t.Run("invalid signature", func(t *testing.T) {
		f.verifier.err = domain.Unauthorized("signature is invalid")
		defer func() { f.verifier.err = nil }()
		w := domain.Wallet{Scheme: domain.SchemeNEP413, Address: "carol.near"}
		if _, _, err := f.svc.LinkWallet(context.Background(), id, f.issue(w)); !errors.Is(err, domain.ErrUnauthorized) {
			t.Fatalf("error = %v, want ErrUnauthorized", err)
		}
		if _, err := f.accounts.FindWalletOwner(context.Background(), w); !errors.Is(err, auth.ErrWalletNotFound) {
			t.Error("wallet must not be linked")
		}
	})
}

func TestAccount_UnlinkWallet(t *testing.T) {
	f := newAccountFixture()
	id := f.accounts.add("alice", testWallet, solanaWallet)
	bob := f.accounts.add("bob", domain.Wallet{Scheme: domain.SchemeNEP413, Address: "bob.near"})
	walletID := func(w domain.Wallet) string { return f.accounts.wallets[w].id }

	if err := f.svc.UnlinkWallet(context.Background(), id, "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("unknown wallet: error = %v, want ErrNotFound", err)
	}
	if err := f.svc.UnlinkWallet(context.Background(), bob, walletID(testWallet)); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("another account's wallet: error = %v, want ErrNotFound", err)
	}
	if err := f.svc.UnlinkWallet(context.Background(), id, walletID(solanaWallet)); err != nil {
		t.Fatalf("UnlinkWallet: %v", err)
	}
	last := walletID(testWallet)
	if err := f.svc.UnlinkWallet(context.Background(), id, last); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("last wallet: error = %v, want ErrConflict", err)
	}
	if n, _ := f.accounts.CountWallets(context.Background(), id); n != 1 {
		t.Errorf("wallets = %d, want the last wallet kept (rolled back)", n)
	}
}
