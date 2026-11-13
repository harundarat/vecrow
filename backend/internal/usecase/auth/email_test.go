package auth_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/harundarat/vecrow/backend/internal/domain"
	"github.com/harundarat/vecrow/backend/internal/usecase/auth"
)

var codePattern = regexp.MustCompile(`\b\d{6}\b`)

// lastCode returns the code in the last email sent to `to`.
func lastCode(t *testing.T, m *fakeMailer, to string) string {
	t.Helper()
	for i := len(m.sent) - 1; i >= 0; i-- {
		if m.sent[i].to == to {
			code := codePattern.FindString(m.sent[i].body)
			if code == "" {
				t.Fatalf("email to %s has no code: %q", to, m.sent[i].body)
			}
			return code
		}
	}
	t.Fatalf("no email sent to %s", to)
	return ""
}

// wrong returns a 6-digit code different from code.
func wrong(code string) string {
	if code == "000000" {
		return "000001"
	}
	return "000000"
}

func TestAccount_ChangeAndVerifyEmail(t *testing.T) {
	ctx := context.Background()
	f := newAccountFixture()
	id := f.accounts.add("alice", testWallet)

	got, err := f.svc.ChangeEmail(ctx, id, "  Alice@Example.COM ")
	if err != nil {
		t.Fatalf("ChangeEmail: %v", err)
	}
	if got.Email != "" || got.PendingEmail != "alice@example.com" {
		t.Fatalf("after ChangeEmail: email %q, pending %q", got.Email, got.PendingEmail)
	}
	if len(f.mailer.sent) != 1 || f.mailer.sent[0].subject != "Your Vecrow verification code" {
		t.Fatalf("sent = %+v", f.mailer.sent)
	}
	code := lastCode(t, f.mailer, "alice@example.com")
	// An unverified email matches nothing.
	if _, err := f.accounts.FindAccountByEmail(ctx, "alice@example.com"); !errors.Is(err, auth.ErrAccountNotFound) {
		t.Fatalf("pending email matched an account: %v", err)
	}

	if _, err := f.svc.VerifyEmail(ctx, id, wrong(code)); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("wrong code: error = %v, want ErrUnauthorized", err)
	}
	if _, err := f.svc.VerifyEmail(ctx, id, "12345"); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("5-digit code: error = %v, want ErrInvalidArgument", err)
	}
	got, err = f.svc.VerifyEmail(ctx, id, code)
	if err != nil {
		t.Fatalf("VerifyEmail: %v", err)
	}
	if got.Email != "alice@example.com" || got.PendingEmail != "" {
		t.Fatalf("after VerifyEmail: email %q, pending %q", got.Email, got.PendingEmail)
	}
	if _, err := f.svc.VerifyEmail(ctx, id, code); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("reused code: error = %v, want ErrUnauthorized", err)
	}

	// Setting the verified email again sends nothing.
	if _, err := f.svc.ChangeEmail(ctx, id, "alice@example.com"); err != nil || len(f.mailer.sent) != 1 {
		t.Fatalf("same email: err %v, %d emails", err, len(f.mailer.sent))
	}
	// A new email stays pending; the verified one keeps working meanwhile.
	got, err = f.svc.ChangeEmail(ctx, id, "alice@new.example")
	if err != nil || got.Email != "alice@example.com" || got.PendingEmail != "alice@new.example" {
		t.Fatalf("change to a new email = %+v, %v", got, err)
	}
	// Removing the email clears both.
	got, err = f.svc.ChangeEmail(ctx, id, "")
	if err != nil || got.Email != "" || got.PendingEmail != "" {
		t.Fatalf("remove email = %+v, %v", got, err)
	}
	if _, err := f.svc.VerifyEmail(ctx, id, lastCode(t, f.mailer, "alice@new.example")); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("code of a removed email: error = %v, want ErrUnauthorized", err)
	}
}

func TestAccount_VerifyEmail_RejectsExpiredAndTooManyAttempts(t *testing.T) {
	ctx := context.Background()
	f := newAccountFixture()
	id := f.accounts.add("alice", testWallet)

	if _, err := f.svc.ChangeEmail(ctx, id, "alice@example.com"); err != nil {
		t.Fatal(err)
	}
	code := lastCode(t, f.mailer, "alice@example.com")
	f.now = fixedNow.Add(auth.OTPTTL)
	if _, err := f.svc.VerifyEmail(ctx, id, code); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("expired code: error = %v, want ErrUnauthorized", err)
	}

	f.now = fixedNow
	if _, err := f.svc.ChangeEmail(ctx, id, "alice@example.com"); err != nil {
		t.Fatal(err)
	}
	code = lastCode(t, f.mailer, "alice@example.com")
	for i := range auth.OTPMaxAttempts - 1 {
		if _, err := f.svc.VerifyEmail(ctx, id, wrong(code)); !errors.Is(err, domain.ErrUnauthorized) {
			t.Fatalf("wrong code %d: error = %v, want ErrUnauthorized", i+1, err)
		}
	}
	// The 5th attempt may still be right; the 6th is refused even if right.
	if _, err := f.svc.VerifyEmail(ctx, id, wrong(code)); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("5th attempt: error = %v, want ErrUnauthorized", err)
	}
	if _, err := f.svc.VerifyEmail(ctx, id, code); !errors.Is(err, domain.ErrTooManyRequests) {
		t.Fatalf("6th attempt: error = %v, want ErrTooManyRequests", err)
	}
	if _, err := f.svc.VerifyEmail(ctx, id, code); !errors.Is(err, domain.ErrTooManyRequests) {
		t.Fatalf("7th attempt: error = %v, want ErrTooManyRequests", err)
	}

	// A new code resets the attempts.
	if _, err := f.svc.ChangeEmail(ctx, id, "alice@example.com"); err != nil {
		t.Fatal(err)
	}
	if got, err := f.svc.VerifyEmail(ctx, id, lastCode(t, f.mailer, "alice@example.com")); err != nil || got.Email != "alice@example.com" {
		t.Fatalf("new code = %+v, %v", got, err)
	}
}

func TestAccount_ChangeEmail_LimitsAndConflicts(t *testing.T) {
	ctx := context.Background()
	f := newAccountFixture()
	id := f.accounts.add("alice", testWallet)
	other := f.accounts.add("bob")
	f.accounts.setEmail(other, "bob@example.com")

	if _, err := f.svc.ChangeEmail(ctx, id, "not-an-email"); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("invalid email: error = %v", err)
	}

	// An email verified by another account still gets a code, so the answer
	// does not tell; verifying it is refused.
	if _, err := f.svc.ChangeEmail(ctx, id, "bob@example.com"); err != nil {
		t.Fatalf("ChangeEmail to a taken email: %v", err)
	}
	if _, err := f.svc.VerifyEmail(ctx, id, lastCode(t, f.mailer, "bob@example.com")); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("taken email: error = %v, want ErrConflict", err)
	}

	// The fixture allows 3 codes per address; bob@ has had one.
	for i := range 2 {
		if _, err := f.svc.ChangeEmail(ctx, id, "bob@example.com"); err != nil {
			t.Fatalf("code %d to bob@: %v", i+2, err)
		}
	}
	if _, err := f.svc.ChangeEmail(ctx, id, "bob@example.com"); !errors.Is(err, domain.ErrTooManyRequests) {
		t.Fatalf("4th code to bob@: error = %v, want ErrTooManyRequests", err)
	}
	// The limit is per address.
	if _, err := f.svc.ChangeEmail(ctx, id, "carol@example.com"); err != nil {
		t.Fatalf("1st code to carol@: %v", err)
	}

	f.mailer.err = errors.New("queue full")
	if _, err := f.svc.ChangeEmail(ctx, id, "dave@example.com"); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("mail failure: error = %v, want ErrUnavailable", err)
	}
}

func TestAccount_LinkWalletInRecoverySessionNotifies(t *testing.T) {
	ctx := context.Background()
	f := newAccountFixture()
	id := f.accounts.add("alice", testWallet)
	f.accounts.setEmail(id, "alice@example.com")

	if _, linked, err := f.svc.LinkWallet(ctx, domain.Session{AccountID: id}, f.issue(solanaWallet)); err != nil || !linked {
		t.Fatalf("LinkWallet with a wallet session = %v, %v", linked, err)
	}
	if len(f.mailer.sent) != 0 {
		t.Fatalf("wallet session sent %+v", f.mailer.sent)
	}

	near := domain.Wallet{Scheme: domain.SchemeNEP413, Address: "alice.near"}
	recovery := domain.Session{AccountID: id, Recovery: true}
	if _, linked, err := f.svc.LinkWallet(ctx, recovery, f.issue(near)); err != nil || !linked {
		t.Fatalf("LinkWallet with a recovery session = %v, %v", linked, err)
	}
	if len(f.mailer.sent) != 1 {
		t.Fatalf("sent %d emails, want 1", len(f.mailer.sent))
	}
	m := f.mailer.sent[0]
	if m.to != "alice@example.com" || m.subject != "A wallet was linked to your Vecrow account" ||
		!strings.Contains(m.body, "alice.near (nep413)") || !strings.Contains(m.body, "alice") {
		t.Fatalf("notification = %+v", m)
	}

	// Linking the same wallet again links nothing and sends nothing.
	if _, linked, err := f.svc.LinkWallet(ctx, recovery, f.issue(near)); err != nil || linked {
		t.Fatalf("second LinkWallet = %v, %v", linked, err)
	}
	// A mail failure does not undo the link.
	f.mailer.err = errors.New("queue full")
	other := domain.Wallet{Scheme: domain.SchemeNEP413, Address: "alice2.near"}
	if _, linked, err := f.svc.LinkWallet(ctx, recovery, f.issue(other)); err != nil || !linked {
		t.Fatalf("LinkWallet with a failing mailer = %v, %v", linked, err)
	}
	if len(f.mailer.sent) != 1 {
		t.Fatalf("sent %d emails, want 1", len(f.mailer.sent))
	}
}

type emailLoginFixture struct {
	accounts *fakeAccounts
	otps     otpMap
	limiter  fakeLimiter
	mailer   *fakeMailer
	sessions sessionMap
	now      time.Time
	logs     *bytes.Buffer
	svc      *auth.EmailLoginService
	account  *auth.AccountService
}

func newEmailLoginFixture() *emailLoginFixture {
	f := &emailLoginFixture{
		accounts: newFakeAccounts(), otps: otpMap{}, limiter: fakeLimiter{}, mailer: &fakeMailer{},
		sessions: sessionMap{}, now: fixedNow, logs: &bytes.Buffer{},
	}
	opts := []auth.Option{
		auth.WithClock(func() time.Time { return f.now }),
		auth.WithLogger(slog.New(slog.NewTextHandler(f.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))),
	}
	otps := auth.NewOTPService(f.otps, f.limiter, f.mailer, auth.OTPConfig{RateLimit: 3, RateWindow: time.Hour}, opts...)
	sessions := auth.NewSessionService(f.sessions, 24*time.Hour, opts...)
	f.svc = auth.NewEmailLoginService(otps, f.accounts, sessions, time.Hour, opts...)
	f.account = auth.NewAccountService(nonceMap{}, &fakeVerifier{}, f.accounts, fakeTx{f.accounts}, otps, f.mailer, opts...)
	return f
}

func TestEmailLogin_OpensRecoverySession(t *testing.T) {
	ctx := context.Background()
	f := newEmailLoginFixture()
	id := f.accounts.add("alice", testWallet)
	f.accounts.setEmail(id, "alice@example.com")

	if err := f.svc.RequestCode(ctx, " Alice@Example.com"); err != nil {
		t.Fatalf("RequestCode: %v", err)
	}
	if len(f.mailer.sent) != 1 || f.mailer.sent[0].subject != "Your Vecrow sign-in code" {
		t.Fatalf("sent = %+v", f.mailer.sent)
	}
	code := lastCode(t, f.mailer, "alice@example.com")

	if _, err := f.svc.Verify(ctx, "alice@example.com", wrong(code)); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("wrong code: error = %v, want ErrUnauthorized", err)
	}
	res, err := f.svc.Verify(ctx, "ALICE@example.com", code)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if res.Account.ID != id || !res.Session.Recovery || res.NewAccount || res.Token == "" {
		t.Fatalf("result = %+v", res)
	}
	if want := fixedNow.Add(time.Hour); !res.Session.ExpiresAt.Equal(want) {
		t.Errorf("recovery session expires at %v, want %v", res.Session.ExpiresAt, want)
	}
	stored := 0
	for _, s := range f.sessions {
		if s.AccountID == id && s.Recovery {
			stored++
		}
	}
	if stored != 1 {
		t.Fatalf("stored recovery sessions = %d, want 1", stored)
	}
	if _, err := f.svc.Verify(ctx, "alice@example.com", code); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("reused code: error = %v, want ErrUnauthorized", err)
	}
	if strings.Contains(f.logs.String(), code) {
		t.Fatalf("logs contain the code: %s", f.logs.String())
	}
}

func TestEmailLogin_UnknownAndUnverifiedEmails(t *testing.T) {
	ctx := context.Background()
	f := newEmailLoginFixture()
	id := f.accounts.add("alice", testWallet)
	if _, err := f.account.ChangeEmail(ctx, id, "alice@example.com"); err != nil {
		t.Fatal(err)
	}
	f.mailer.sent = nil
	accountsBefore := len(f.accounts.accounts)

	// Neither an unknown nor a pending (unverified) email gets a code, and
	// both get the same answer as a verified one.
	for _, email := range []string{"nobody@example.com", "alice@example.com"} {
		if err := f.svc.RequestCode(ctx, email); err != nil {
			t.Fatalf("RequestCode(%s) = %v, want nil", email, err)
		}
		if _, err := f.svc.Verify(ctx, email, "123456"); !errors.Is(err, domain.ErrUnauthorized) {
			t.Fatalf("Verify(%s) error = %v, want ErrUnauthorized", email, err)
		}
	}
	if len(f.mailer.sent) != 0 {
		t.Fatalf("sent %+v", f.mailer.sent)
	}
	if len(f.accounts.accounts) != accountsBefore {
		t.Fatal("an unknown email created an account")
	}
	if err := f.svc.RequestCode(ctx, "not an email"); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("invalid email: error = %v", err)
	}
}

func TestEmailLogin_RateLimitIsPerEmailAndBlind(t *testing.T) {
	ctx := context.Background()
	f := newEmailLoginFixture()
	id := f.accounts.add("alice", testWallet)
	f.accounts.setEmail(id, "alice@example.com")

	// Known and unknown addresses hit the limit (3 in the fixture) alike.
	for _, email := range []string{"alice@example.com", "nobody@example.com"} {
		for i := range 3 {
			if err := f.svc.RequestCode(ctx, email); err != nil {
				t.Fatalf("%s code %d: %v", email, i+1, err)
			}
		}
		if err := f.svc.RequestCode(ctx, email); !errors.Is(err, domain.ErrTooManyRequests) {
			t.Fatalf("%s 4th code: error = %v, want ErrTooManyRequests", email, err)
		}
	}
	if len(f.mailer.sent) != 3 {
		t.Fatalf("sent %d emails, want 3", len(f.mailer.sent))
	}
	// A failing mailer gets the same answer as success.
	g := newEmailLoginFixture()
	gid := g.accounts.add("bob", testWallet)
	g.accounts.setEmail(gid, "bob@example.com")
	g.mailer.err = errors.New("queue full")
	if err := g.svc.RequestCode(ctx, "bob@example.com"); err != nil {
		t.Fatalf("RequestCode with a failing mailer = %v, want nil", err)
	}
}

func TestEmailLogin_RejectsExpiredOverLimitAndMovedCodes(t *testing.T) {
	ctx := context.Background()
	f := newEmailLoginFixture()
	id := f.accounts.add("alice", testWallet)
	f.accounts.setEmail(id, "alice@example.com")

	// Expired.
	if err := f.svc.RequestCode(ctx, "alice@example.com"); err != nil {
		t.Fatal(err)
	}
	code := lastCode(t, f.mailer, "alice@example.com")
	f.now = fixedNow.Add(auth.OTPTTL + time.Second)
	if _, err := f.svc.Verify(ctx, "alice@example.com", code); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("expired code: error = %v, want ErrUnauthorized", err)
	}

	// Over the attempt limit.
	f.now = fixedNow
	if err := f.svc.RequestCode(ctx, "alice@example.com"); err != nil {
		t.Fatal(err)
	}
	code = lastCode(t, f.mailer, "alice@example.com")
	for range auth.OTPMaxAttempts {
		if _, err := f.svc.Verify(ctx, "alice@example.com", wrong(code)); !errors.Is(err, domain.ErrUnauthorized) {
			t.Fatalf("wrong code: error = %v", err)
		}
	}
	if _, err := f.svc.Verify(ctx, "alice@example.com", code); !errors.Is(err, domain.ErrTooManyRequests) {
		t.Fatalf("right code after the limit: error = %v, want ErrTooManyRequests", err)
	}

	// The email was removed from the account after the code was sent.
	if err := f.svc.RequestCode(ctx, "alice@example.com"); err != nil {
		t.Fatal(err)
	}
	code = lastCode(t, f.mailer, "alice@example.com")
	if _, err := f.account.ChangeEmail(ctx, id, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Verify(ctx, "alice@example.com", code); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("code of a removed email: error = %v, want ErrUnauthorized", err)
	}
}

func TestEmailLogin_CodesDoNotCrossPurposes(t *testing.T) {
	ctx := context.Background()
	f := newEmailLoginFixture()
	id := f.accounts.add("alice", testWallet)
	f.accounts.setEmail(id, "alice@example.com")

	// A verification code for a new email is not a sign-in code.
	if _, err := f.account.ChangeEmail(ctx, id, "alice@new.example"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Verify(ctx, "alice@new.example", lastCode(t, f.mailer, "alice@new.example")); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("verification code used to sign in: error = %v, want ErrUnauthorized", err)
	}
	// And a sign-in code does not verify an email.
	if err := f.svc.RequestCode(ctx, "alice@example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.account.VerifyEmail(ctx, id, lastCode(t, f.mailer, "alice@example.com")); err == nil {
		t.Fatal("sign-in code verified an email")
	}
}

func TestSession_RecoveryIsCappedBySessionTTL(t *testing.T) {
	store := sessionMap{}
	svc := auth.NewSessionService(store, 30*time.Minute, auth.WithClock(func() time.Time { return fixedNow }))
	token, sess, err := svc.CreateRecovery(context.Background(), "acct-1", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if !sess.Recovery || !sess.ExpiresAt.Equal(fixedNow.Add(30*time.Minute)) {
		t.Fatalf("session = %+v", sess)
	}
	got, err := svc.Authenticate(context.Background(), token)
	if err != nil || !got.Recovery {
		t.Fatalf("Authenticate = %+v, %v", got, err)
	}
	_, wallet, err := svc.Create(context.Background(), "acct-1")
	if err != nil || wallet.Recovery {
		t.Fatalf("Create = %+v, %v", wallet, err)
	}
}

// After recovery the user links a new wallet; it then logs in like any
// wallet, with a full session of the same account (R-22).
func TestEmailLogin_WalletLinkedInRecoveryLogsInNormally(t *testing.T) {
	ctx := context.Background()
	f := newEmailLoginFixture()
	id := f.accounts.add("alice", testWallet)
	f.accounts.setEmail(id, "alice@example.com")
	if err := f.svc.RequestCode(ctx, "alice@example.com"); err != nil {
		t.Fatal(err)
	}
	recovered, err := f.svc.Verify(ctx, "alice@example.com", lastCode(t, f.mailer, "alice@example.com"))
	if err != nil || !recovered.Session.Recovery {
		t.Fatalf("recovery login = %+v, %v", recovered, err)
	}

	nonces := nonceMap{}
	proof := func(w domain.Wallet, b byte) auth.Proof {
		v := nonceValue(b)
		nonces[v] = domain.Nonce{Value: v, Wallet: w, Message: "sign me"}
		return auth.Proof{Nonce: v, Signature: "sig"}
	}
	otps := auth.NewOTPService(f.otps, f.limiter, f.mailer, auth.OTPConfig{RateLimit: 3, RateWindow: time.Hour})
	accounts := auth.NewAccountService(nonces, &fakeVerifier{}, f.accounts, fakeTx{f.accounts}, otps, f.mailer)
	newWallet := domain.Wallet{Scheme: domain.SchemeNEP413, Address: "alice-new.near"}
	if _, linked, err := accounts.LinkWallet(ctx, recovered.Session, proof(newWallet, 1)); err != nil || !linked {
		t.Fatalf("LinkWallet in the recovery session = %v, %v", linked, err)
	}

	login := auth.NewLoginService(nonces, &fakeVerifier{}, f.accounts, fakeTx{f.accounts},
		auth.NewSessionService(f.sessions, 24*time.Hour))
	res, err := login.Login(ctx, proof(newWallet, 2))
	if err != nil {
		t.Fatalf("Login with the new wallet: %v", err)
	}
	if res.Account.ID != id || res.NewAccount || res.Session.Recovery || res.Token == "" {
		t.Fatalf("login with the new wallet = %+v", res)
	}
}
