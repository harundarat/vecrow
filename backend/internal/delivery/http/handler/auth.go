package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/harundarat/vecrow/backend/internal/delivery/http/middleware"
	"github.com/harundarat/vecrow/backend/internal/delivery/http/response"
	"github.com/harundarat/vecrow/backend/internal/domain"
	"github.com/harundarat/vecrow/backend/internal/usecase/auth"
)

// NonceIssuer is the use case behind POST /auth/nonce.
type NonceIssuer interface {
	Issue(ctx context.Context, scheme, address string) (domain.Nonce, error)
	Recipient() string
}

// LoginUseCase is the use case behind POST /auth/verify and /auth/logout.
type LoginUseCase interface {
	Login(ctx context.Context, p auth.Proof) (auth.LoginResult, error)
	Logout(ctx context.Context, token string) error
}

// EmailLoginUseCase is the use case behind POST /auth/email/otp and
// /auth/email/verify (R-22).
type EmailLoginUseCase interface {
	RequestCode(ctx context.Context, email string) error
	Verify(ctx context.Context, email, code string) (auth.LoginResult, error)
}

// AuthHandler serves the /auth endpoints.
type AuthHandler struct {
	nonces NonceIssuer
	logins LoginUseCase
	emails EmailLoginUseCase
}

func NewAuthHandler(nonces NonceIssuer, logins LoginUseCase, emails EmailLoginUseCase) *AuthHandler {
	return &AuthHandler{nonces: nonces, logins: logins, emails: emails}
}

type issueNonceRequest struct {
	Scheme  string `json:"scheme" binding:"required"`
	Address string `json:"address" binding:"required"`
}

type issueNonceResponse struct {
	Nonce     string    `json:"nonce"`
	Message   string    `json:"message"`
	Recipient string    `json:"recipient"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (h *AuthHandler) IssueNonce(c *gin.Context) {
	var req issueNonceRequest
	if !bindJSON(c, &req) {
		return
	}
	nonce, err := h.nonces.Issue(c.Request.Context(), req.Scheme, req.Address)
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, issueNonceResponse{
		Nonce:     nonce.Value,
		Message:   nonce.Message,
		Recipient: h.nonces.Recipient(),
		ExpiresAt: nonce.ExpiresAt,
	})
}

// proofRequest is a wallet's signature over the message of a nonce
// (POST /auth/verify and POST /me/wallets).
type proofRequest struct {
	Nonce     string `json:"nonce" binding:"required"`
	Signature string `json:"signature" binding:"required"`
	PublicKey string `json:"public_key"`
}

func (r proofRequest) proof() auth.Proof {
	return auth.Proof{Nonce: r.Nonce, Signature: r.Signature, PublicKey: r.PublicKey}
}

type verifyResponse struct {
	Token      string          `json:"token"`
	ExpiresAt  time.Time       `json:"expires_at"`
	NewAccount bool            `json:"new_account"`
	Account    accountResponse `json:"account"`
}

// Verify logs a wallet in (creating its account on first login) and returns
// a bearer session token.
func (h *AuthHandler) Verify(c *gin.Context) {
	var req proofRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := h.logins.Login(c.Request.Context(), req.proof())
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, verifyResponse{
		Token:      res.Token,
		ExpiresAt:  res.Session.ExpiresAt,
		NewAccount: res.NewAccount,
		Account:    newAccountResponse(res.Account),
	})
}

// Logout revokes the session of the request's bearer token. It runs behind
// middleware.RequireSession.
func (h *AuthHandler) Logout(c *gin.Context) {
	token, _ := middleware.BearerToken(c.Request)
	if err := h.logins.Logout(c.Request.Context(), token); err != nil {
		response.Error(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

type emailCodeRequest struct {
	Email string `json:"email" binding:"required"`
}

type emailCodeResponse struct {
	Message          string `json:"message"`
	ExpiresInSeconds int    `json:"expires_in_seconds"`
}

// RequestEmailCode emails a recovery sign-in code (R-22). It answers 202 with
// the same body whether or not the email belongs to an account.
func (h *AuthHandler) RequestEmailCode(c *gin.Context) {
	var req emailCodeRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := h.emails.RequestCode(c.Request.Context(), req.Email); err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusAccepted, emailCodeResponse{
		Message:          "If this email is verified on an account, a sign-in code was sent to it.",
		ExpiresInSeconds: int(auth.OTPTTL.Seconds()),
	})
}

type emailVerifyRequest struct {
	Email string `json:"email" binding:"required"`
	Code  string `json:"code" binding:"required"`
}

type emailVerifyResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	// Recovery is always true: the session can only view the account and
	// link or unlink wallets.
	Recovery bool            `json:"recovery"`
	Account  accountResponse `json:"account"`
}

// VerifyEmailCode opens a recovery session with an emailed code (R-22).
func (h *AuthHandler) VerifyEmailCode(c *gin.Context) {
	var req emailVerifyRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := h.emails.Verify(c.Request.Context(), req.Email, req.Code)
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, emailVerifyResponse{
		Token:     res.Token,
		ExpiresAt: res.Session.ExpiresAt,
		Recovery:  res.Session.Recovery,
		Account:   newAccountResponse(res.Account),
	})
}
