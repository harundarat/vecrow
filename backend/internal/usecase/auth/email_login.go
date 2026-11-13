package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/harundarat/vecrow/backend/internal/domain"
)

// EmailLoginService logs an account in with a code sent to its verified
// email (R-22). The session is recovery-only: it can view the account and
// link or unlink wallets, and nothing else. It never restores rights in the
// contract (D-08).
type EmailLoginService struct {
	otps        *OTPService
	accounts    AccountRepository
	sessions    *SessionService
	recoveryTTL time.Duration
	rt          runtime
}

func NewEmailLoginService(otps *OTPService, accounts AccountRepository, sessions *SessionService,
	recoveryTTL time.Duration, opts ...Option) *EmailLoginService {
	return &EmailLoginService{otps: otps, accounts: accounts, sessions: sessions, recoveryTTL: recoveryTTL, rt: newRuntime(opts)}
}

// RequestCode emails a sign-in code to email when it is the verified email of
// an account. The result is the same whether or not it is, and unknown
// addresses never create an account.
func (s *EmailLoginService) RequestCode(ctx context.Context, email string) error {
	e, err := domain.ParseEmail(email)
	if err != nil {
		return err
	}
	// Counted for every address, known or not, so the limit says nothing
	// about whether the address has an account.
	if err := s.otps.allow(ctx, e); err != nil {
		return err
	}
	accountID, err := s.accounts.FindAccountByEmail(ctx, e)
	if errors.Is(err, ErrAccountNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("auth: find account by email: %w", err)
	}
	code, err := s.otps.issue(ctx, otpPurposeLogin, e, OTPRecord{AccountID: accountID, Email: e})
	if err != nil {
		return err
	}
	subject, body := loginEmailMessage(code)
	if err := s.otps.mailer.Send(ctx, e, subject, body); err != nil {
		// Answered like an unknown address; the user can ask again.
		s.rt.log.ErrorContext(ctx, "sign-in code email not queued",
			slog.String("account_id", accountID), slog.String("error", err.Error()))
		return nil
	}
	s.rt.log.InfoContext(ctx, "sign-in code emailed", slog.String("account_id", accountID))
	return nil
}

// Verify checks the code sent to email and opens a recovery session.
func (s *EmailLoginService) Verify(ctx context.Context, email, code string) (LoginResult, error) {
	e, err := domain.ParseEmail(email)
	if err != nil {
		return LoginResult{}, err
	}
	rec, err := s.otps.check(ctx, otpPurposeLogin, e, code)
	if err != nil {
		return LoginResult{}, err
	}
	// The email may have been removed from the account, or verified by
	// another one, since the code was sent.
	owner, err := s.accounts.FindAccountByEmail(ctx, e)
	if errors.Is(err, ErrAccountNotFound) || (err == nil && owner != rec.AccountID) {
		return LoginResult{}, errWrongCode
	}
	if err != nil {
		return LoginResult{}, fmt.Errorf("auth: find account by email: %w", err)
	}
	account, err := s.accounts.GetAccount(ctx, owner)
	if err != nil {
		return LoginResult{}, fmt.Errorf("auth: get account: %w", err)
	}
	token, sess, err := s.sessions.CreateRecovery(ctx, owner, s.recoveryTTL)
	if err != nil {
		return LoginResult{}, err
	}
	s.rt.log.InfoContext(ctx, "recovery login succeeded", slog.String("account_id", owner))
	return LoginResult{Token: token, Session: sess, Account: account}, nil
}
