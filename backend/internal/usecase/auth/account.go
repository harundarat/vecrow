package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/harundarat/vecrow/backend/internal/domain"
	"github.com/harundarat/vecrow/backend/internal/usecase"
)

// AccountService manages the signed-in account: profile and linked wallets.
type AccountService struct {
	nonces   NonceStore
	verifier SignatureVerifier
	accounts AccountRepository
	tx       usecase.Transactor
	rt       runtime
}

func NewAccountService(nonces NonceStore, verifier SignatureVerifier, accounts AccountRepository,
	tx usecase.Transactor, opts ...Option) *AccountService {
	return &AccountService{nonces: nonces, verifier: verifier, accounts: accounts, tx: tx, rt: newRuntime(opts)}
}

// Get returns the account with its wallets.
func (s *AccountService) Get(ctx context.Context, accountID string) (domain.Account, error) {
	account, err := s.accounts.GetAccount(ctx, accountID)
	if errors.Is(err, ErrAccountNotFound) {
		return domain.Account{}, domain.NotFound("account not found")
	}
	if err != nil {
		return domain.Account{}, fmt.Errorf("auth: get account: %w", err)
	}
	return account, nil
}

// ChangeUsername sets a new, unique username.
func (s *AccountService) ChangeUsername(ctx context.Context, accountID, username string) (domain.Account, error) {
	u, err := domain.ParseUsername(username)
	if err != nil {
		return domain.Account{}, err
	}
	err = s.accounts.UpdateUsername(ctx, accountID, u)
	switch {
	case errors.Is(err, ErrUsernameTaken):
		return domain.Account{}, domain.Conflict("username %s is already taken", u)
	case errors.Is(err, ErrAccountNotFound):
		return domain.Account{}, domain.NotFound("account not found")
	case err != nil:
		return domain.Account{}, fmt.Errorf("auth: update username: %w", err)
	}
	s.rt.log.InfoContext(ctx, "username changed", slog.String("account_id", accountID))
	return s.Get(ctx, accountID)
}

// LinkWallet links the wallet that signed p to the account. linked is false
// when the wallet was already linked to this account (the call is
// idempotent); a wallet owned by another account is rejected.
func (s *AccountService) LinkWallet(ctx context.Context, accountID string, p Proof) (account domain.Account, linked bool, err error) {
	wallet, err := proveWallet(ctx, s.nonces, s.verifier, p)
	if err != nil {
		return domain.Account{}, false, err
	}
	linked, err = s.linkWallet(ctx, accountID, wallet)
	if errors.Is(err, ErrWalletNotFound) {
		// The owner unlinked the wallet between the insert and the lookup.
		s.rt.log.DebugContext(ctx, "wallet unlinked during link, retrying",
			slog.String("account_id", accountID))
		linked, err = s.linkWallet(ctx, accountID, wallet)
	}
	if err != nil {
		return domain.Account{}, false, err
	}
	if linked {
		s.rt.log.InfoContext(ctx, "wallet linked",
			slog.String("account_id", accountID), slog.String("scheme", string(wallet.Scheme)))
	}
	account, err = s.Get(ctx, accountID)
	return account, linked, err
}

func (s *AccountService) linkWallet(ctx context.Context, accountID string, wallet domain.Wallet) (bool, error) {
	linked, err := s.accounts.LinkWallet(ctx, accountID, wallet)
	if err != nil {
		return false, fmt.Errorf("auth: link wallet: %w", err)
	}
	if linked {
		return true, nil
	}
	owner, err := s.accounts.FindWalletOwner(ctx, wallet)
	if err != nil {
		return false, fmt.Errorf("auth: find wallet owner: %w", err)
	}
	if owner != accountID {
		return false, domain.Conflict("wallet is already linked to another account")
	}
	return false, nil
}

// UnlinkWallet removes a wallet from the account. The last wallet cannot be
// removed: without it the account could no longer log in.
func (s *AccountService) UnlinkWallet(ctx context.Context, accountID, walletID string) error {
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		// The lock serialises concurrent unlinks, which could otherwise each
		// see two wallets and together remove both.
		if err := s.accounts.LockAccount(ctx, accountID); err != nil {
			if errors.Is(err, ErrAccountNotFound) {
				return domain.NotFound("account not found")
			}
			return fmt.Errorf("auth: lock account: %w", err)
		}
		deleted, err := s.accounts.UnlinkWallet(ctx, accountID, walletID)
		if err != nil {
			return fmt.Errorf("auth: unlink wallet: %w", err)
		}
		if !deleted {
			return domain.NotFound("wallet not found")
		}
		remaining, err := s.accounts.CountWallets(ctx, accountID)
		if err != nil {
			return fmt.Errorf("auth: count wallets: %w", err)
		}
		if remaining == 0 {
			return domain.Conflict("cannot unlink the last wallet of an account") // rolls back
		}
		return nil
	})
	if err != nil {
		return err
	}
	// After the commit, so a rolled-back unlink is never reported.
	s.rt.log.InfoContext(ctx, "wallet unlinked",
		slog.String("account_id", accountID), slog.String("wallet_id", walletID))
	return nil
}
