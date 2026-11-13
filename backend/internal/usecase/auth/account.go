package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/harundarat/vecrow/backend/internal/domain"
	"github.com/harundarat/vecrow/backend/internal/usecase"
)

// AccountService manages the signed-in account: profile, email and linked
// wallets.
type AccountService struct {
	nonces   NonceStore
	verifier SignatureVerifier
	accounts AccountRepository
	tx       usecase.Transactor
	otps     *OTPService
	mailer   usecase.Mailer
	rt       runtime
}

func NewAccountService(nonces NonceStore, verifier SignatureVerifier, accounts AccountRepository,
	tx usecase.Transactor, otps *OTPService, mailer usecase.Mailer, opts ...Option) *AccountService {
	return &AccountService{
		nonces: nonces, verifier: verifier, accounts: accounts, tx: tx,
		otps: otps, mailer: mailer, rt: newRuntime(opts),
	}
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

// ChangeEmail sets a new email for the account. It stays pending, and is not
// used for invitations or recovery, until VerifyEmail gets the code emailed
// to it. An empty email removes the account's email.
func (s *AccountService) ChangeEmail(ctx context.Context, accountID, email string) (domain.Account, error) {
	if strings.TrimSpace(email) == "" {
		return s.removeEmail(ctx, accountID)
	}
	e, err := domain.ParseEmail(email)
	if err != nil {
		return domain.Account{}, err
	}
	account, err := s.Get(ctx, accountID)
	if err != nil {
		return domain.Account{}, err
	}
	if account.Email == e {
		// Already verified: drop a pending change, if any.
		if account.PendingEmail == "" {
			return account, nil
		}
		if err := s.setPendingEmail(ctx, accountID, ""); err != nil {
			return domain.Account{}, err
		}
		if err := s.otps.discard(ctx, otpPurposeEmail, accountID); err != nil {
			return domain.Account{}, err
		}
		return s.Get(ctx, accountID)
	}
	if err := s.otps.allow(ctx, e); err != nil {
		return domain.Account{}, err
	}
	if err := s.setPendingEmail(ctx, accountID, e); err != nil {
		return domain.Account{}, err
	}
	code, err := s.otps.issue(ctx, otpPurposeEmail, accountID, OTPRecord{AccountID: accountID, Email: e})
	if err != nil {
		return domain.Account{}, err
	}
	subject, body := verifyEmailMessage(code)
	if err := s.mailer.Send(ctx, e, subject, body); err != nil {
		s.rt.log.ErrorContext(ctx, "verification email not queued",
			slog.String("account_id", accountID), slog.String("error", err.Error()))
		return domain.Account{}, domain.Unavailable("the verification email could not be sent; try again")
	}
	s.rt.log.InfoContext(ctx, "email verification code emailed", slog.String("account_id", accountID))
	return s.Get(ctx, accountID)
}

// VerifyEmail checks the code emailed by ChangeEmail and makes the pending
// email the account's verified email.
func (s *AccountService) VerifyEmail(ctx context.Context, accountID, code string) (domain.Account, error) {
	rec, err := s.otps.check(ctx, otpPurposeEmail, accountID, code)
	if err != nil {
		return domain.Account{}, err
	}
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		confirmed, err := s.accounts.ConfirmEmail(ctx, accountID, rec.Email)
		switch {
		case errors.Is(err, ErrEmailTaken):
			return domain.Conflict("this email is already verified by another account")
		case err != nil:
			return fmt.Errorf("auth: confirm email: %w", err)
		case !confirmed:
			return domain.Conflict("the email was changed after the code was sent; request a new code")
		}
		return nil
	})
	if err != nil {
		return domain.Account{}, err
	}
	s.rt.log.InfoContext(ctx, "email verified", slog.String("account_id", accountID))
	return s.Get(ctx, accountID)
}

func (s *AccountService) removeEmail(ctx context.Context, accountID string) (domain.Account, error) {
	err := s.accounts.ClearEmail(ctx, accountID)
	if errors.Is(err, ErrAccountNotFound) {
		return domain.Account{}, domain.NotFound("account not found")
	}
	if err != nil {
		return domain.Account{}, fmt.Errorf("auth: clear email: %w", err)
	}
	if err := s.otps.discard(ctx, otpPurposeEmail, accountID); err != nil {
		return domain.Account{}, err
	}
	s.rt.log.InfoContext(ctx, "email removed", slog.String("account_id", accountID))
	return s.Get(ctx, accountID)
}

func (s *AccountService) setPendingEmail(ctx context.Context, accountID, email string) error {
	err := s.accounts.SetPendingEmail(ctx, accountID, email)
	if errors.Is(err, ErrAccountNotFound) {
		return domain.NotFound("account not found")
	}
	if err != nil {
		return fmt.Errorf("auth: set pending email: %w", err)
	}
	return nil
}

// LinkWallet links the wallet that signed p to the account of sess. linked
// is false when the wallet was already linked to this account (the call is
// idempotent); a wallet owned by another account is rejected. A wallet linked
// in a recovery session is reported to the account's email (R-22).
func (s *AccountService) LinkWallet(ctx context.Context, sess domain.Session, p Proof) (account domain.Account, linked bool, err error) {
	accountID := sess.AccountID
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
			slog.String("account_id", accountID), slog.String("scheme", string(wallet.Scheme)),
			slog.Bool("recovery_session", sess.Recovery))
	}
	account, err = s.Get(ctx, accountID)
	if err != nil {
		return domain.Account{}, false, err
	}
	if linked && sess.Recovery {
		s.notifyWalletLinked(ctx, account, wallet)
	}
	return account, linked, nil
}

// notifyWalletLinked emails the account about a wallet linked in a recovery
// session. The wallet is linked already, so a failure is logged, not
// returned.
func (s *AccountService) notifyWalletLinked(ctx context.Context, account domain.Account, w domain.Wallet) {
	if account.Email == "" {
		s.rt.log.WarnContext(ctx, "wallet linked in a recovery session of an account without email",
			slog.String("account_id", account.ID))
		return
	}
	subject, body := walletLinkedMessage(account, w, s.rt.now())
	if err := s.mailer.Send(ctx, account.Email, subject, body); err != nil {
		s.rt.log.ErrorContext(ctx, "wallet-linked notification not queued",
			slog.String("account_id", account.ID), slog.String("error", err.Error()))
		return
	}
	s.rt.log.InfoContext(ctx, "wallet-linked notification emailed", slog.String("account_id", account.ID))
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
