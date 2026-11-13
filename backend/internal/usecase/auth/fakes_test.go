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

func (f *fakeAccounts) FindAccountByEmail(_ context.Context, email string) (string, error) {
	for id, a := range f.accounts {
		if email != "" && a.Email == email {
			return id, nil
		}
	}
	return "", auth.ErrAccountNotFound
}

func (f *fakeAccounts) SetPendingEmail(_ context.Context, id, email string) error {
	a, ok := f.accounts[id]
	if !ok {
		return auth.ErrAccountNotFound
	}
	a.PendingEmail = email
	f.accounts[id] = a
	return nil
}

func (f *fakeAccounts) ConfirmEmail(_ context.Context, id, email string) (bool, error) {
	a, ok := f.accounts[id]
	if !ok || a.PendingEmail == "" || a.PendingEmail != email {
		return false, nil
	}
	for otherID, other := range f.accounts {
		if otherID != id && other.Email == email {
			return false, auth.ErrEmailTaken
		}
	}
	a.Email, a.PendingEmail = email, ""
	f.accounts[id] = a
	return true, nil
}

func (f *fakeAccounts) ClearEmail(_ context.Context, id string) error {
	a, ok := f.accounts[id]
	if !ok {
		return auth.ErrAccountNotFound
	}
	a.Email, a.PendingEmail = "", ""
	f.accounts[id] = a
	return nil
}

// setEmail gives an account a verified email.
func (f *fakeAccounts) setEmail(id, email string) {
	a := f.accounts[id]
	a.Email = email
	f.accounts[id] = a
}

// otpMap is an in-memory auth.OTPStore; it does not expire codes, which the
// service checks against their ExpiresAt.
type otpMap map[string]*otpEntry

type otpEntry struct {
	rec      auth.OTPRecord
	present  bool
	attempts int
}

func otpMapKey(purpose, subject string) string { return purpose + "|" + subject }

func (m otpMap) Save(_ context.Context, purpose, subject string, rec auth.OTPRecord, _ time.Duration) error {
	m[otpMapKey(purpose, subject)] = &otpEntry{rec: rec, present: true}
	return nil
}

func (m otpMap) Get(_ context.Context, purpose, subject string) (auth.OTPRecord, error) {
	e := m[otpMapKey(purpose, subject)]
	if e == nil || !e.present {
		return auth.OTPRecord{}, auth.ErrOTPNotFound
	}
	return e.rec, nil
}

func (m otpMap) Consume(ctx context.Context, purpose, subject string) (auth.OTPRecord, error) {
	rec, err := m.Get(ctx, purpose, subject)
	if err == nil {
		delete(m, otpMapKey(purpose, subject))
	}
	return rec, err
}

func (m otpMap) CountAttempt(_ context.Context, purpose, subject string, _ time.Duration) (int, error) {
	k := otpMapKey(purpose, subject)
	if m[k] == nil {
		m[k] = &otpEntry{}
	}
	m[k].attempts++
	return m[k].attempts, nil
}

func (m otpMap) Delete(_ context.Context, purpose, subject string) error {
	if e := m[otpMapKey(purpose, subject)]; e != nil {
		e.present, e.rec = false, auth.OTPRecord{}
	}
	return nil
}

// fakeLimiter counts events per key without windows.
type fakeLimiter map[string]int

func (f fakeLimiter) Allow(_ context.Context, key string, limit int, _ time.Duration) (bool, error) {
	f[key]++
	return f[key] <= limit, nil
}

type sentMail struct{ to, subject, body string }

// fakeMailer records emails; it fails with err when set.
type fakeMailer struct {
	sent []sentMail
	err  error
}

func (f *fakeMailer) Send(_ context.Context, to, subject, body string) error {
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, sentMail{to: to, subject: subject, body: body})
	return nil
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

type guestMap map[string]domain.GuestToken

func (m guestMap) Save(_ context.Context, key string, g domain.GuestToken, _ time.Duration) error {
	m[key] = g
	return nil
}

func (m guestMap) Get(_ context.Context, key string) (domain.GuestToken, error) {
	g, ok := m[key]
	if !ok {
		return domain.GuestToken{}, auth.ErrGuestTokenNotFound
	}
	return g, nil
}
