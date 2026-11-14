package auth

import (
	"context"
	"errors"
	"time"

	"github.com/harundarat/vecrow/backend/internal/domain"
)

// Errors returned by the ports. They are not domain errors: the use cases
// translate them into client-facing ones.
var (
	ErrNonceNotFound   = errors.New("auth: nonce not found")
	ErrSessionNotFound = errors.New("auth: session not found")
	ErrAccountNotFound = errors.New("auth: account not found")
	ErrWalletNotFound  = errors.New("auth: wallet not found")
	ErrUsernameTaken   = errors.New("auth: username taken")
	ErrEmailTaken      = errors.New("auth: email taken")
	ErrOTPNotFound     = errors.New("auth: one-time code not found")
)

// NonceStore keeps issued nonces until they are used or expire.
type NonceStore interface {
	Save(ctx context.Context, nonce domain.Nonce, ttl time.Duration) error
	// Consume returns the nonce stored under value and deletes it in the same
	// step, so each nonce can be used once. It returns ErrNonceNotFound for
	// unknown, expired or already used nonces.
	Consume(ctx context.Context, value string) (domain.Nonce, error)
}

// SessionStore keeps sessions under a key derived from the token; the token
// itself is never stored.
type SessionStore interface {
	Save(ctx context.Context, key string, s domain.Session, ttl time.Duration) error
	// Get returns ErrSessionNotFound for unknown or expired sessions.
	Get(ctx context.Context, key string) (domain.Session, error)
	Delete(ctx context.Context, key string) error
}

// AccountRepository persists accounts and their wallets. Calls made with a
// ctx from usecase.Transactor.WithinTx join that transaction.
type AccountRepository interface {
	// GetAccount returns the account with its wallets, or ErrAccountNotFound.
	GetAccount(ctx context.Context, id string) (domain.Account, error)
	// FindWalletOwner returns the ID of the account w is linked to, or
	// ErrWalletNotFound.
	FindWalletOwner(ctx context.Context, w domain.Wallet) (string, error)
	// CreateAccount inserts an account and returns its ID. It returns
	// ErrUsernameTaken, without failing the transaction, when the username is
	// in use.
	CreateAccount(ctx context.Context, username string) (string, error)
	// LinkWallet links w to the account. It reports false, without failing
	// the transaction, when w is already linked to any account.
	LinkWallet(ctx context.Context, accountID string, w domain.Wallet) (bool, error)
	// UpdateUsername returns ErrUsernameTaken or ErrAccountNotFound.
	UpdateUsername(ctx context.Context, id, username string) error
	// LockAccount locks the account until the transaction ends, or returns
	// ErrAccountNotFound.
	LockAccount(ctx context.Context, id string) error
	CountWallets(ctx context.Context, accountID string) (int, error)
	// UnlinkWallet deletes the account's wallet and reports whether it existed.
	UnlinkWallet(ctx context.Context, accountID, walletID string) (bool, error)
	// FindAccountByEmail returns the ID of the account whose verified email
	// is email, or ErrAccountNotFound. Pending emails never match.
	FindAccountByEmail(ctx context.Context, email string) (string, error)
	// SetPendingEmail records the email waiting for its code; "" clears it.
	// It returns ErrAccountNotFound.
	SetPendingEmail(ctx context.Context, accountID, email string) error
	// ConfirmEmail makes the pending email the verified email if it is still
	// email, and reports false when it is not. It returns ErrEmailTaken when
	// another account has verified the same email.
	ConfirmEmail(ctx context.Context, accountID, email string) (bool, error)
	// ClearEmail removes the verified and the pending email.
	ClearEmail(ctx context.Context, accountID string) error
}

// OTPRecord is an issued one-time code (R-14, R-22). The code itself is not
// stored: only an HMAC of it keyed by a random salt.
type OTPRecord struct {
	// AccountID is the account the code was issued for.
	AccountID string
	// Email is the address the code was sent to.
	Email     string
	Salt      []byte
	MAC       []byte
	ExpiresAt time.Time
}

// OTPStore keeps one-time codes by purpose ("email" verification or "login")
// and subject (an account ID or an email), with a count of attempts.
type OTPStore interface {
	// Save stores rec for ttl, replacing an earlier code of the subject and
	// resetting its attempt count.
	Save(ctx context.Context, purpose, subject string, rec OTPRecord, ttl time.Duration) error
	// Get returns ErrOTPNotFound for unknown or expired codes.
	Get(ctx context.Context, purpose, subject string) (OTPRecord, error)
	// Consume returns the code and deletes it and its attempt count in one
	// step, so a code is used once; ErrOTPNotFound if it is already gone.
	Consume(ctx context.Context, purpose, subject string) (OTPRecord, error)
	// CountAttempt counts one attempt at the subject's code and returns the
	// attempts so far. A new count expires after ttl.
	CountAttempt(ctx context.Context, purpose, subject string, ttl time.Duration) (int, error)
	// Delete removes the code, keeping its attempt count.
	Delete(ctx context.Context, purpose, subject string) error
}

// RateLimiter counts events per key in fixed windows.
type RateLimiter interface {
	// Allow counts one event for key and reports whether the events of the
	// current window are within limit.
	Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error)
}

// AccessKeyStatus says how a public key relates to a NEAR account.
type AccessKeyStatus = domain.AccessKeyStatus

const (
	AccessKeyFullAccess = domain.AccessKeyFullAccess
	AccessKeyLimited    = domain.AccessKeyLimited
	AccessKeyMissing    = domain.AccessKeyMissing
	AccountMissing      = domain.AccountMissing
)

// NEARAccessKeys looks up access keys on the NEAR chain.
type NEARAccessKeys interface {
	Status(ctx context.Context, accountID, publicKey string) (AccessKeyStatus, error)
}

// Proof is a wallet's signature over the message of a nonce.
type Proof struct {
	Nonce     string
	Signature string
	// PublicKey is the signing key, required for NEAR wallets
	// ("ed25519:<base58>"); EVM and Solana keys follow from the address.
	PublicKey string
}

// SignatureVerifier checks that p.Signature is n.Wallet's signature of
// n.Message. Client mistakes are returned as domain errors.
type SignatureVerifier interface {
	Verify(ctx context.Context, n domain.Nonce, p Proof) error
}
