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
