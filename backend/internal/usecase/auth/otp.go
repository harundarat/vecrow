package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"math/big"
	"time"

	"github.com/harundarat/vecrow/backend/internal/domain"
	"github.com/harundarat/vecrow/backend/internal/usecase"
)

// One-time codes as the spec fixes them (R-14, R-22).
const (
	OTPDigits      = 6
	OTPTTL         = 10 * time.Minute
	OTPMaxAttempts = 5
)

// Purposes of one-time codes; a code of one purpose never passes for another.
const (
	otpPurposeEmail = "email" // verify an account's new email; subject: account ID
	otpPurposeLogin = "login" // recovery login; subject: the email
)

const otpSaltSize = 16

// errWrongCode is the answer to a wrong, expired, used or unknown code, so
// the answer does not tell which.
var errWrongCode = domain.Unauthorized("the code is wrong or has expired; request a new one")

// OTPConfig limits how many codes one email address receives.
type OTPConfig struct {
	// RateLimit codes per email address in each RateWindow.
	RateLimit  int
	RateWindow time.Duration
}

// OTPService issues, emails and checks one-time codes. Codes are stored as an
// HMAC under a random salt and compared in constant time; they are never
// logged.
type OTPService struct {
	store   OTPStore
	limiter RateLimiter
	mailer  usecase.Mailer
	cfg     OTPConfig
	rt      runtime
}

func NewOTPService(store OTPStore, limiter RateLimiter, mailer usecase.Mailer, cfg OTPConfig, opts ...Option) *OTPService {
	return &OTPService{store: store, limiter: limiter, mailer: mailer, cfg: cfg, rt: newRuntime(opts)}
}

// allow counts one code sent to email and rejects it beyond the rate limit.
func (s *OTPService) allow(ctx context.Context, email string) error {
	ok, err := s.limiter.Allow(ctx, "otp:"+email, s.cfg.RateLimit, s.cfg.RateWindow)
	if err != nil {
		return fmt.Errorf("auth: otp rate limit: %w", err)
	}
	if !ok {
		return domain.TooManyRequests("too many codes were requested for this email; try again later")
	}
	return nil
}

// issue stores a new code for (purpose, subject), replacing an earlier one,
// and returns it.
func (s *OTPService) issue(ctx context.Context, purpose, subject string, rec OTPRecord) (string, error) {
	n, err := rand.Int(s.rt.random, big.NewInt(1_000_000))
	if err != nil {
		return "", fmt.Errorf("auth: generate code: %w", err)
	}
	code := fmt.Sprintf("%0*d", OTPDigits, n.Int64())
	rec.Salt = make([]byte, otpSaltSize)
	if _, err := io.ReadFull(s.rt.random, rec.Salt); err != nil {
		return "", fmt.Errorf("auth: generate code salt: %w", err)
	}
	rec.MAC = otpMAC(rec.Salt, purpose, subject, code)
	rec.ExpiresAt = s.rt.now().Add(OTPTTL).UTC()
	if err := s.store.Save(ctx, purpose, subject, rec, OTPTTL); err != nil {
		return "", fmt.Errorf("auth: save code: %w", err)
	}
	return code, nil
}

// check counts an attempt at the code of (purpose, subject) and, if it
// matches, consumes it. After OTPMaxAttempts attempts the code is deleted.
func (s *OTPService) check(ctx context.Context, purpose, subject, code string) (OTPRecord, error) {
	if !validCode(code) {
		return OTPRecord{}, domain.InvalidArgument("code must be %d digits", OTPDigits)
	}
	attempts, err := s.store.CountAttempt(ctx, purpose, subject, OTPTTL)
	if err != nil {
		return OTPRecord{}, fmt.Errorf("auth: count code attempt: %w", err)
	}
	if attempts > OTPMaxAttempts {
		if err := s.store.Delete(ctx, purpose, subject); err != nil {
			return OTPRecord{}, fmt.Errorf("auth: delete code: %w", err)
		}
		return OTPRecord{}, domain.TooManyRequests("too many wrong codes; request a new code")
	}
	rec, err := s.store.Get(ctx, purpose, subject)
	if errors.Is(err, ErrOTPNotFound) {
		return OTPRecord{}, errWrongCode
	}
	if err != nil {
		return OTPRecord{}, fmt.Errorf("auth: get code: %w", err)
	}
	if !s.rt.now().Before(rec.ExpiresAt) || !hmac.Equal(rec.MAC, otpMAC(rec.Salt, purpose, subject, code)) {
		return OTPRecord{}, errWrongCode
	}
	// Consuming after the comparison makes the code single-use even when two
	// requests with the right code race.
	rec, err = s.store.Consume(ctx, purpose, subject)
	if errors.Is(err, ErrOTPNotFound) {
		return OTPRecord{}, errWrongCode
	}
	if err != nil {
		return OTPRecord{}, fmt.Errorf("auth: consume code: %w", err)
	}
	return rec, nil
}

// discard deletes the code of (purpose, subject), if any.
func (s *OTPService) discard(ctx context.Context, purpose, subject string) error {
	if err := s.store.Delete(ctx, purpose, subject); err != nil {
		return fmt.Errorf("auth: delete code: %w", err)
	}
	return nil
}

func validCode(code string) bool {
	if len(code) != OTPDigits {
		return false
	}
	for _, c := range code {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// otpMAC binds the code to its purpose and subject.
func otpMAC(salt []byte, purpose, subject, code string) []byte {
	m := hmac.New(sha256.New, salt)
	m.Write([]byte(purpose + "\x00" + subject + "\x00" + code))
	return m.Sum(nil)
}

// Emails. Plain text; the code is on a line of its own.

func verifyEmailMessage(code string) (subject, body string) {
	return "Your Vecrow verification code",
		"Use this code to verify your email for your Vecrow account:\n\n" +
			"    " + code + "\n\n" +
			"It expires in 10 minutes. If you did not add this email to a Vecrow account, ignore this message.\n"
}

func loginEmailMessage(code string) (subject, body string) {
	return "Your Vecrow sign-in code",
		"Use this code to sign in to your Vecrow account with your email:\n\n" +
			"    " + code + "\n\n" +
			"It expires in 10 minutes. A sign-in with an email code can only view the account " +
			"and link or unlink wallets.\n\n" +
			"If you did not ask for this code, ignore this message: nobody can sign in without it.\n"
}

func walletLinkedMessage(account domain.Account, w domain.Wallet, at time.Time) (subject, body string) {
	return "A wallet was linked to your Vecrow account",
		"The wallet " + w.Address + " (" + string(w.Scheme) + ") was linked to your Vecrow account " +
			account.Username + " on " + at.UTC().Format("2006-01-02 15:04 UTC") +
			", in a session signed in with an email code.\n\n" +
			"If this was not you, sign in with a wallet you still control, unlink this wallet, and " +
			"secure your email account. A linked wallet gains no rights over escrows: those stay " +
			"with the wallets named in each escrow.\n"
}
