package auth

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/harundarat/vecrow/backend/internal/domain"
	"github.com/harundarat/vecrow/backend/internal/usecase"
)

// maxUsernameAttempts bounds retries when a generated username is taken.
const maxUsernameAttempts = 5

// errWalletRace means another login linked the same wallet first.
var errWalletRace = errors.New("auth: wallet linked concurrently")

// LoginResult is the outcome of a successful login.
type LoginResult struct {
	Token   string
	Session domain.Session
	Account domain.Account
	// NewAccount is true when this login created the account.
	NewAccount bool
}

// LoginService logs a wallet in, creating its account on first login.
type LoginService struct {
	nonces   NonceStore
	verifier SignatureVerifier
	accounts AccountRepository
	tx       usecase.Transactor
	sessions *SessionService
	rt       runtime
}

func NewLoginService(nonces NonceStore, verifier SignatureVerifier, accounts AccountRepository,
	tx usecase.Transactor, sessions *SessionService, opts ...Option) *LoginService {
	return &LoginService{
		nonces: nonces, verifier: verifier, accounts: accounts,
		tx: tx, sessions: sessions, rt: newRuntime(opts),
	}
}

// Login verifies the wallet's signature over a nonce and opens a session for
// the account the wallet belongs to.
func (s *LoginService) Login(ctx context.Context, p Proof) (LoginResult, error) {
	wallet, err := proveWallet(ctx, s.nonces, s.verifier, p)
	if err != nil {
		return LoginResult{}, err
	}
	accountID, created, err := s.findOrCreateAccount(ctx, wallet)
	if err != nil {
		return LoginResult{}, err
	}
	account, err := s.accounts.GetAccount(ctx, accountID)
	if err != nil {
		return LoginResult{}, fmt.Errorf("auth: get account: %w", err)
	}
	token, sess, err := s.sessions.Create(ctx, accountID)
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{Token: token, Session: sess, Account: account, NewAccount: created}, nil
}

// Logout ends the session of token.
func (s *LoginService) Logout(ctx context.Context, token string) error {
	return s.sessions.Revoke(ctx, token)
}

func (s *LoginService) findOrCreateAccount(ctx context.Context, w domain.Wallet) (id string, created bool, err error) {
	id, err = s.accounts.FindWalletOwner(ctx, w)
	if err == nil {
		return id, false, nil
	}
	if !errors.Is(err, ErrWalletNotFound) {
		return "", false, fmt.Errorf("auth: find wallet owner: %w", err)
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		var err error
		if id, err = s.createAccount(ctx); err != nil {
			return err
		}
		linked, err := s.accounts.LinkWallet(ctx, id, w)
		if err != nil {
			return fmt.Errorf("auth: link wallet: %w", err)
		}
		if !linked {
			return errWalletRace // rolls back the new account
		}
		return nil
	})
	if errors.Is(err, errWalletRace) {
		// A concurrent first login with this wallet created the account.
		if id, err = s.accounts.FindWalletOwner(ctx, w); err != nil {
			return "", false, fmt.Errorf("auth: find wallet owner: %w", err)
		}
		return id, false, nil
	}
	if err != nil {
		return "", false, err
	}
	return id, true, nil
}

// createAccount inserts an account with a generated username, which the user
// can change later.
func (s *LoginService) createAccount(ctx context.Context) (string, error) {
	for range maxUsernameAttempts {
		username, err := generateUsername(s.rt.random)
		if err != nil {
			return "", err
		}
		id, err := s.accounts.CreateAccount(ctx, username)
		if errors.Is(err, ErrUsernameTaken) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("auth: create account: %w", err)
		}
		return id, nil
	}
	return "", fmt.Errorf("auth: no free username after %d attempts", maxUsernameAttempts)
}

const usernameAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

// generateUsername returns "user_" and 8 random characters (36^8 ≈ 2.8e12
// combinations). The slight modulo bias does not matter: usernames are
// public, not secrets.
func generateUsername(r io.Reader) (string, error) {
	buf := make([]byte, 8)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", fmt.Errorf("auth: generate username: %w", err)
	}
	for i, b := range buf {
		buf[i] = usernameAlphabet[int(b)%len(usernameAlphabet)]
	}
	return "user_" + string(buf), nil
}
