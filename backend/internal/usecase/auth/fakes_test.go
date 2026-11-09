package auth_test

import (
	"context"
	"fmt"
	"maps"
	"time"

	"github.com/harundarat/vecrow/backend/internal/domain"
	"github.com/harundarat/vecrow/backend/internal/usecase/auth"
)

// nonceMap is an in-memory auth.NonceStore.
type nonceMap map[string]domain.Nonce

func (m nonceMap) Save(_ context.Context, n domain.Nonce, _ time.Duration) error {
	m[n.Value] = n
	return nil
}

func (m nonceMap) Consume(_ context.Context, value string) (domain.Nonce, error) {
	n, ok := m[value]
	if !ok {
		return domain.Nonce{}, auth.ErrNonceNotFound
	}
	delete(m, value)
	return n, nil
}

// fakeVerifier accepts every signature unless err is set.
type fakeVerifier struct {
	err   error
	calls int
}

func (f *fakeVerifier) Verify(context.Context, domain.Nonce, auth.Proof) error {
	f.calls++
	return f.err
}

// sessionMap is an in-memory auth.SessionStore.
type sessionMap map[string]domain.Session

func (m sessionMap) Save(_ context.Context, key string, s domain.Session, _ time.Duration) error {
	m[key] = s
	return nil
}

func (m sessionMap) Get(_ context.Context, key string) (domain.Session, error) {
	s, ok := m[key]
	if !ok {
		return domain.Session{}, auth.ErrSessionNotFound
	}
	return s, nil
}

func (m sessionMap) Delete(_ context.Context, key string) error {
	delete(m, key)
	return nil
}

type walletRow struct {
	id        string
	accountID string
}

// fakeAccounts is an in-memory auth.AccountRepository.
type fakeAccounts struct {
	accounts map[string]domain.Account // without wallets
	wallets  map[domain.Wallet]walletRow
	nextID   int
	// hideOwnerOnce makes the next FindWalletOwner miss, simulating a
	// concurrent login that links the wallet after the first lookup.
	hideOwnerOnce bool
	err           error // returned by FindWalletOwner when set
}

func newFakeAccounts() *fakeAccounts {
	return &fakeAccounts{accounts: map[string]domain.Account{}, wallets: map[domain.Wallet]walletRow{}}
}

func (f *fakeAccounts) id(prefix string) string {
	f.nextID++
	return fmt.Sprintf("%s-%d", prefix, f.nextID)
}

// add creates an account owning the wallets and returns its ID.
func (f *fakeAccounts) add(username string, wallets ...domain.Wallet) string {
	id := f.id("acct")
	f.accounts[id] = domain.Account{ID: id, Username: username}
	for _, w := range wallets {
		f.wallets[w] = walletRow{id: f.id("wallet"), accountID: id}
	}
	return id
}

func (f *fakeAccounts) walletsOf(accountID string) []domain.Wallet {
	var out []domain.Wallet
	for w, row := range f.wallets {
		if row.accountID == accountID {
			out = append(out, w)
		}
	}
	return out
}

func (f *fakeAccounts) GetAccount(_ context.Context, id string) (domain.Account, error) {
	a, ok := f.accounts[id]
	if !ok {
		return domain.Account{}, auth.ErrAccountNotFound
	}
	for w, row := range f.wallets {
		if row.accountID == id {
			a.Wallets = append(a.Wallets, domain.LinkedWallet{ID: row.id, Wallet: w})
		}
	}
	return a, nil
}

func (f *fakeAccounts) FindWalletOwner(_ context.Context, w domain.Wallet) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	if f.hideOwnerOnce {
		f.hideOwnerOnce = false
		return "", auth.ErrWalletNotFound
	}
	row, ok := f.wallets[w]
	if !ok {
		return "", auth.ErrWalletNotFound
	}
	return row.accountID, nil
}

func (f *fakeAccounts) CreateAccount(_ context.Context, username string) (string, error) {
	for _, a := range f.accounts {
		if a.Username == username {
			return "", auth.ErrUsernameTaken
		}
	}
	id := f.id("acct")
	f.accounts[id] = domain.Account{ID: id, Username: username}
	return id, nil
}

func (f *fakeAccounts) LinkWallet(_ context.Context, accountID string, w domain.Wallet) (bool, error) {
	if _, ok := f.wallets[w]; ok {
		return false, nil
	}
	f.wallets[w] = walletRow{id: f.id("wallet"), accountID: accountID}
	return true, nil
}

func (f *fakeAccounts) UpdateUsername(_ context.Context, id, username string) error {
	a, ok := f.accounts[id]
	if !ok {
		return auth.ErrAccountNotFound
	}
	for otherID, other := range f.accounts {
		if other.Username == username && otherID != id {
			return auth.ErrUsernameTaken
		}
	}
	a.Username = username
	f.accounts[id] = a
	return nil
}

func (f *fakeAccounts) LockAccount(_ context.Context, id string) error {
	if _, ok := f.accounts[id]; !ok {
		return auth.ErrAccountNotFound
	}
	return nil
}

func (f *fakeAccounts) CountWallets(_ context.Context, accountID string) (int, error) {
	return len(f.walletsOf(accountID)), nil
}

func (f *fakeAccounts) UnlinkWallet(_ context.Context, accountID, walletID string) (bool, error) {
	for w, row := range f.wallets {
		if row.id == walletID && row.accountID == accountID {
			delete(f.wallets, w)
			return true, nil
		}
	}
	return false, nil
}

// fakeTx rolls fakeAccounts back when fn fails, like a real transaction.
type fakeTx struct {
	accounts *fakeAccounts
}

func (t fakeTx) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	accounts, wallets, nextID := maps.Clone(t.accounts.accounts), maps.Clone(t.accounts.wallets), t.accounts.nextID
	if err := fn(ctx); err != nil {
		t.accounts.accounts, t.accounts.wallets, t.accounts.nextID = accounts, wallets, nextID
		return err
	}
	return nil
}
