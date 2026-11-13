package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/harundarat/vecrow/backend/internal/domain"
	"github.com/harundarat/vecrow/backend/internal/usecase/auth"
)

const (
	validToken    = "valid-token-0123456789abcdefghijklmnopqrstu"
	recoveryToken = "recovery-token-0123456789abcdefghijklmnopq"
)

// fakeSessions accepts validToken as a wallet session and recoveryToken as a
// recovery session (R-22), both for account acct-1.
type fakeSessions struct{}

func (fakeSessions) Authenticate(_ context.Context, token string) (domain.Session, error) {
	switch token {
	case validToken:
		return domain.Session{AccountID: "acct-1"}, nil
	case recoveryToken:
		return domain.Session{AccountID: "acct-1", Recovery: true}, nil
	}
	return domain.Session{}, domain.Unauthorized("session is invalid or expired")
}

type fakeLogin struct {
	result    auth.LoginResult
	err       error
	gotProof  auth.Proof
	loggedOut []string
}

func (f *fakeLogin) Login(_ context.Context, p auth.Proof) (auth.LoginResult, error) {
	f.gotProof = p
	return f.result, f.err
}

func (f *fakeLogin) Logout(_ context.Context, token string) error {
	f.loggedOut = append(f.loggedOut, token)
	return nil
}

type fakeEmailLogin struct {
	result    auth.LoginResult
	err       error
	requested []string
	gotEmail  string
	gotCode   string
}

func (f *fakeEmailLogin) RequestCode(_ context.Context, email string) error {
	f.requested = append(f.requested, email)
	return f.err
}

func (f *fakeEmailLogin) Verify(_ context.Context, email, code string) (auth.LoginResult, error) {
	f.gotEmail, f.gotCode = email, code
	return f.result, f.err
}

type fakeAccountUseCase struct {
	account    domain.Account
	err        error
	linked     bool
	gotAccount string
	gotArg     string
	gotSession domain.Session
	calls      []string
}

func (f *fakeAccountUseCase) Get(_ context.Context, id string) (domain.Account, error) {
	f.gotAccount = id
	return f.account, f.err
}

func (f *fakeAccountUseCase) ChangeUsername(_ context.Context, id, username string) (domain.Account, error) {
	f.gotAccount, f.gotArg = id, username
	f.calls = append(f.calls, "username:"+username)
	return f.account, f.err
}

func (f *fakeAccountUseCase) ChangeEmail(_ context.Context, id, email string) (domain.Account, error) {
	f.gotAccount, f.gotArg = id, email
	f.calls = append(f.calls, "email:"+email)
	return f.account, f.err
}

func (f *fakeAccountUseCase) VerifyEmail(_ context.Context, id, code string) (domain.Account, error) {
	f.gotAccount, f.gotArg = id, code
	return f.account, f.err
}

func (f *fakeAccountUseCase) LinkWallet(_ context.Context, sess domain.Session, p auth.Proof) (domain.Account, bool, error) {
	f.gotAccount, f.gotArg, f.gotSession = sess.AccountID, p.Nonce, sess
	return f.account, f.linked, f.err
}

func (f *fakeAccountUseCase) UnlinkWallet(_ context.Context, id, walletID string) error {
	f.gotAccount, f.gotArg = id, walletID
	return f.err
}

var testAccount = domain.Account{
	ID:        "acct-1",
	Username:  "user_11111111",
	CreatedAt: time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC),
	Wallets: []domain.LinkedWallet{{
		ID:     "wallet-1",
		Wallet: domain.Wallet{Scheme: domain.SchemeEIP191, Address: "0xf39fd6e51aad88f6f4ce6ab8827279cfffb92266"},
	}},
}

const bearer = "Bearer " + validToken

func TestIssueNonce_ReturnsMessageAndRecipient(t *testing.T) {
	srv := newTestServer(t, fakeIssuer{nonce: domain.Nonce{Value: "abc", Message: "sign this"}})
	rec := srv.do(http.MethodPost, "/api/v1/auth/nonce", `{"scheme":"nep413","address":"alice.near"}`)
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != http.StatusCreated {
		t.Fatalf("status %d body %s", rec.Code, rec.Body)
	}
	if body["message"] != "sign this" || body["recipient"] != "localhost:5173" {
		t.Errorf("body = %v", body)
	}
}

func TestVerify(t *testing.T) {
	srv := newTestServer(t, fakeIssuer{})
	srv.login.result = auth.LoginResult{
		Token:      validToken,
		Session:    domain.Session{AccountID: "acct-1", ExpiresAt: time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)},
		Account:    testAccount,
		NewAccount: true,
	}

	rec := srv.do(http.MethodPost, "/api/v1/auth/verify",
		`{"nonce":"n1","signature":"sig","public_key":"ed25519:abc"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body)
	}
	if want := (auth.Proof{Nonce: "n1", Signature: "sig", PublicKey: "ed25519:abc"}); srv.login.gotProof != want {
		t.Errorf("proof = %+v, want %+v", srv.login.gotProof, want)
	}
	var body struct {
		Token      string `json:"token"`
		NewAccount bool   `json:"new_account"`
		Account    struct {
			Username string `json:"username"`
			Wallets  []struct {
				ID, Scheme, Address string
			} `json:"wallets"`
		} `json:"account"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Token != validToken || !body.NewAccount || body.Account.Username != "user_11111111" ||
		len(body.Account.Wallets) != 1 || body.Account.Wallets[0].Scheme != "eip191" {
		t.Errorf("body = %s", rec.Body)
	}
	if strings.Contains(srv.logs.String(), validToken) {
		t.Error("the session token was logged")
	}
}

func TestVerify_ClientErrors(t *testing.T) {
	srv := newTestServer(t, fakeIssuer{})
	rec := srv.do(http.MethodPost, "/api/v1/auth/verify", `{"nonce":"n1"}`)
	if rec.Code != http.StatusBadRequest || decodeError(t, rec).Error.Message != "signature is required" {
		t.Errorf("missing signature: status %d body %s", rec.Code, rec.Body)
	}

	srv = newTestServer(t, fakeIssuer{})
	srv.login.err = domain.Unauthorized("signature is invalid")
	rec = srv.do(http.MethodPost, "/api/v1/auth/verify", `{"nonce":"n1","signature":"bad"}`)
	if rec.Code != http.StatusUnauthorized || decodeError(t, rec).Error.Code != "unauthorized" {
		t.Errorf("invalid signature: status %d body %s", rec.Code, rec.Body)
	}
}

func TestSessionRoutes_RequireBearerToken(t *testing.T) {
	routes := [][2]string{
		{http.MethodPost, "/api/v1/auth/logout"},
		{http.MethodGet, "/api/v1/me"},
		{http.MethodPatch, "/api/v1/me"},
		{http.MethodPost, "/api/v1/me/wallets"},
		{http.MethodDelete, "/api/v1/me/wallets/wallet-1"},
	}
	for _, rt := range routes {
		for name, header := range map[string]string{
			"missing":      "",
			"wrong scheme": "Basic " + validToken,
			"unknown":      "Bearer nope",
		} {
			t.Run(rt[0]+" "+rt[1]+" "+name, func(t *testing.T) {
				srv := newTestServer(t, fakeIssuer{})
				var rec = srv.do(rt[0], rt[1], "")
				if header != "" {
					rec = srv.do(rt[0], rt[1], "", "Authorization", header)
				}
				if rec.Code != http.StatusUnauthorized || decodeError(t, rec).Error.Code != "unauthorized" {
					t.Fatalf("status %d body %s", rec.Code, rec.Body)
				}
				if !strings.HasPrefix(rec.Header().Get("WWW-Authenticate"), "Bearer") {
					t.Errorf("WWW-Authenticate = %q", rec.Header().Get("WWW-Authenticate"))
				}
			})
		}
	}
}

func TestLogout(t *testing.T) {
	srv := newTestServer(t, fakeIssuer{})
	rec := srv.do(http.MethodPost, "/api/v1/auth/logout", "", "Authorization", bearer)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status %d body %s", rec.Code, rec.Body)
	}
	if len(srv.login.loggedOut) != 1 || srv.login.loggedOut[0] != validToken {
		t.Errorf("logged out = %v", srv.login.loggedOut)
	}
	if strings.Contains(srv.logs.String(), validToken) {
		t.Error("the session token was logged")
	}
}

func TestMe(t *testing.T) {
	t.Run("get", func(t *testing.T) {
		srv := newTestServer(t, fakeIssuer{})
		srv.accounts.account = testAccount
		rec := srv.do(http.MethodGet, "/api/v1/me", "", "Authorization", bearer)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"username":"user_11111111"`) {
			t.Fatalf("status %d body %s", rec.Code, rec.Body)
		}
		if srv.accounts.gotAccount != "acct-1" {
			t.Errorf("account = %q, want the session's account", srv.accounts.gotAccount)
		}
		if srv.accessLog(t)["account_id"] != "acct-1" {
			t.Error("access log should carry account_id")
		}
	})
	t.Run("change username conflict", func(t *testing.T) {
		srv := newTestServer(t, fakeIssuer{})
		srv.accounts.err = domain.Conflict("username bob is already taken")
		rec := srv.do(http.MethodPatch, "/api/v1/me", `{"username":"bob"}`, "Authorization", bearer)
		if rec.Code != http.StatusConflict || srv.accounts.gotArg != "bob" {
			t.Fatalf("status %d body %s", rec.Code, rec.Body)
		}
	})
	t.Run("link wallet", func(t *testing.T) {
		for linked, want := range map[bool]int{true: http.StatusCreated, false: http.StatusOK} {
			srv := newTestServer(t, fakeIssuer{})
			srv.accounts.account, srv.accounts.linked = testAccount, linked
			rec := srv.do(http.MethodPost, "/api/v1/me/wallets", `{"nonce":"n2","signature":"sig"}`, "Authorization", bearer)
			if rec.Code != want || srv.accounts.gotArg != "n2" {
				t.Errorf("linked=%v: status %d, want %d; body %s", linked, rec.Code, want, rec.Body)
			}
		}
	})
	t.Run("unlink wallet", func(t *testing.T) {
		srv := newTestServer(t, fakeIssuer{})
		rec := srv.do(http.MethodDelete, "/api/v1/me/wallets/wallet-9", "", "Authorization", bearer)
		if rec.Code != http.StatusNoContent || srv.accounts.gotArg != "wallet-9" {
			t.Fatalf("status %d body %s", rec.Code, rec.Body)
		}

		srv = newTestServer(t, fakeIssuer{})
		srv.accounts.err = domain.Conflict("cannot unlink the last wallet of an account")
		if rec := srv.do(http.MethodDelete, "/api/v1/me/wallets/wallet-1", "", "Authorization", bearer); rec.Code != http.StatusConflict {
			t.Fatalf("last wallet: status %d body %s", rec.Code, rec.Body)
		}
	})
}

const recoveryBearer = "Bearer " + recoveryToken

func TestRequestEmailCode(t *testing.T) {
	srv := newTestServer(t, fakeIssuer{})
	rec := srv.do(http.MethodPost, "/api/v1/auth/email/otp", `{"email":"alice@example.com"}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status %d body %s", rec.Code, rec.Body)
	}
	var body struct {
		Message          string `json:"message"`
		ExpiresInSeconds int    `json:"expires_in_seconds"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.ExpiresInSeconds != 600 || !strings.Contains(body.Message, "If this email is verified") {
		t.Errorf("body = %s", rec.Body)
	}
	if len(srv.emailLogin.requested) != 1 || srv.emailLogin.requested[0] != "alice@example.com" {
		t.Errorf("requested = %v", srv.emailLogin.requested)
	}

	if rec := srv.do(http.MethodPost, "/api/v1/auth/email/otp", `{}`); rec.Code != http.StatusBadRequest {
		t.Errorf("no email: status %d", rec.Code)
	}
	srv.emailLogin.err = domain.TooManyRequests("too many codes")
	rec = srv.do(http.MethodPost, "/api/v1/auth/email/otp", `{"email":"alice@example.com"}`)
	if rec.Code != http.StatusTooManyRequests || decodeError(t, rec).Error.Code != "too_many_requests" {
		t.Errorf("rate limited: status %d body %s", rec.Code, rec.Body)
	}
}

func TestVerifyEmailCode(t *testing.T) {
	srv := newTestServer(t, fakeIssuer{})
	expires := time.Date(2026, 11, 8, 13, 0, 0, 0, time.UTC)
	srv.emailLogin.result = auth.LoginResult{
		Token:   recoveryToken,
		Session: domain.Session{AccountID: "acct-1", ExpiresAt: expires, Recovery: true},
		Account: testAccount,
	}
	rec := srv.do(http.MethodPost, "/api/v1/auth/email/verify", `{"email":"alice@example.com","code":"123456"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body)
	}
	var body struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
		Recovery  bool      `json:"recovery"`
		Account   struct {
			Username string `json:"username"`
		} `json:"account"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Token != recoveryToken || !body.Recovery || !body.ExpiresAt.Equal(expires) || body.Account.Username != "user_11111111" {
		t.Errorf("body = %s", rec.Body)
	}
	if srv.emailLogin.gotEmail != "alice@example.com" || srv.emailLogin.gotCode != "123456" {
		t.Errorf("got %q %q", srv.emailLogin.gotEmail, srv.emailLogin.gotCode)
	}
	if strings.Contains(srv.logs.String(), "123456") || strings.Contains(srv.logs.String(), recoveryToken) {
		t.Error("the code or the token was logged")
	}

	srv.emailLogin.err = domain.Unauthorized("the code is wrong or has expired")
	rec = srv.do(http.MethodPost, "/api/v1/auth/email/verify", `{"email":"alice@example.com","code":"123456"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("wrong code: status %d", rec.Code)
	}
	if rec := srv.do(http.MethodPost, "/api/v1/auth/email/verify", `{"email":"alice@example.com"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("no code: status %d", rec.Code)
	}
}

// A recovery session from an email code can view the account and link or
// unlink wallets, and nothing else (R-22).
func TestRecoverySession_IsLimited(t *testing.T) {
	rejected := [][2]string{
		{http.MethodPost, "/api/v1/escrows"},
		{http.MethodPost, "/api/v1/escrows/esc-1/counterparty/approve"},
		{http.MethodPost, "/api/v1/escrows/esc-1/counterparty/reject"},
		{http.MethodPatch, "/api/v1/me"},
		{http.MethodPost, "/api/v1/me/email/verify"},
		{http.MethodGet, "/api/v1/me/escrows"},
		{http.MethodGet, "/api/v1/escrows/esc-1"},
		{http.MethodPost, "/api/v1/escrows/esc-1/cancel"},
		{http.MethodPost, "/api/v1/escrows/esc-1/quote"},
		{http.MethodGet, "/api/v1/escrows/esc-1/funding"},
	}
	for _, rt := range rejected {
		t.Run(rt[0]+" "+rt[1], func(t *testing.T) {
			srv := newTestServer(t, fakeIssuer{})
			rec := srv.do(rt[0], rt[1], `{}`, "Authorization", recoveryBearer)
			if rec.Code != http.StatusForbidden || decodeError(t, rec).Error.Code != "forbidden" {
				t.Fatalf("status %d body %s", rec.Code, rec.Body)
			}
			if len(srv.accounts.calls) != 0 || srv.accounts.gotAccount != "" {
				t.Fatalf("the use case ran: %+v", srv.accounts)
			}
		})
	}

	srv := newTestServer(t, fakeIssuer{})
	srv.accounts.account, srv.accounts.linked = testAccount, true
	if rec := srv.do(http.MethodGet, "/api/v1/me", "", "Authorization", recoveryBearer); rec.Code != http.StatusOK {
		t.Errorf("GET /me: status %d body %s", rec.Code, rec.Body)
	}
	rec := srv.do(http.MethodPost, "/api/v1/me/wallets", `{"nonce":"n2","signature":"sig"}`, "Authorization", recoveryBearer)
	if rec.Code != http.StatusCreated || !srv.accounts.gotSession.Recovery || srv.accounts.gotSession.AccountID != "acct-1" {
		t.Errorf("POST /me/wallets: status %d, session %+v", rec.Code, srv.accounts.gotSession)
	}
	if rec := srv.do(http.MethodDelete, "/api/v1/me/wallets/wallet-1", "", "Authorization", recoveryBearer); rec.Code != http.StatusNoContent {
		t.Errorf("DELETE /me/wallets/wallet-1: status %d body %s", rec.Code, rec.Body)
	}
	if rec := srv.do(http.MethodPost, "/api/v1/auth/logout", "", "Authorization", recoveryBearer); rec.Code != http.StatusNoContent {
		t.Errorf("POST /auth/logout: status %d body %s", rec.Code, rec.Body)
	}

	// A wallet session links with a non-recovery session.
	rec = srv.do(http.MethodPost, "/api/v1/me/wallets", `{"nonce":"n3","signature":"sig"}`, "Authorization", bearer)
	if rec.Code != http.StatusCreated || srv.accounts.gotSession.Recovery {
		t.Errorf("wallet session: status %d, session %+v", rec.Code, srv.accounts.gotSession)
	}
}

func TestMe_Email(t *testing.T) {
	t.Run("change email", func(t *testing.T) {
		srv := newTestServer(t, fakeIssuer{})
		account := testAccount
		account.PendingEmail = "alice@example.com"
		srv.accounts.account = account
		rec := srv.do(http.MethodPatch, "/api/v1/me", `{"email":"alice@example.com"}`, "Authorization", bearer)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body)
		}
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body["email"] != nil || body["pending_email"] != "alice@example.com" {
			t.Errorf("body = %s", rec.Body)
		}
		if strings.Join(srv.accounts.calls, ",") != "email:alice@example.com" {
			t.Errorf("calls = %v", srv.accounts.calls)
		}
	})
	t.Run("username and email", func(t *testing.T) {
		srv := newTestServer(t, fakeIssuer{})
		srv.accounts.account = testAccount
		rec := srv.do(http.MethodPatch, "/api/v1/me", `{"username":"alice","email":""}`, "Authorization", bearer)
		if rec.Code != http.StatusOK || strings.Join(srv.accounts.calls, ",") != "username:alice,email:" {
			t.Fatalf("status %d, calls %v", rec.Code, srv.accounts.calls)
		}
	})
	t.Run("nothing to change", func(t *testing.T) {
		srv := newTestServer(t, fakeIssuer{})
		if rec := srv.do(http.MethodPatch, "/api/v1/me", `{}`, "Authorization", bearer); rec.Code != http.StatusBadRequest {
			t.Fatalf("status %d", rec.Code)
		}
	})
	t.Run("verify email", func(t *testing.T) {
		srv := newTestServer(t, fakeIssuer{})
		account := testAccount
		account.Email = "alice@example.com"
		srv.accounts.account = account
		rec := srv.do(http.MethodPost, "/api/v1/me/email/verify", `{"code":"654321"}`, "Authorization", bearer)
		if rec.Code != http.StatusOK || srv.accounts.gotArg != "654321" {
			t.Fatalf("status %d body %s", rec.Code, rec.Body)
		}
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body["email"] != "alice@example.com" || body["pending_email"] != nil {
			t.Errorf("body = %s", rec.Body)
		}
		if rec := srv.do(http.MethodPost, "/api/v1/me/email/verify", `{}`, "Authorization", bearer); rec.Code != http.StatusBadRequest {
			t.Errorf("no code: status %d", rec.Code)
		}
	})
}
