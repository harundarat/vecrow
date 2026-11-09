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

// AuthHandler serves the /auth endpoints.
type AuthHandler struct {
	nonces NonceIssuer
	logins LoginUseCase
}

func NewAuthHandler(nonces NonceIssuer, logins LoginUseCase) *AuthHandler {
	return &AuthHandler{nonces: nonces, logins: logins}
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
