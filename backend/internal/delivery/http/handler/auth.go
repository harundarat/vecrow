package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/harundarat/vecrow/backend/internal/delivery/http/response"
	"github.com/harundarat/vecrow/backend/internal/domain"
)

// NonceIssuer is the use case behind POST /auth/nonce.
type NonceIssuer interface {
	Issue(ctx context.Context, scheme, address string) (domain.Nonce, error)
}

// AuthHandler serves the /auth endpoints.
type AuthHandler struct {
	nonces NonceIssuer
}

func NewAuthHandler(nonces NonceIssuer) *AuthHandler {
	return &AuthHandler{nonces: nonces}
}

type issueNonceRequest struct {
	Scheme  string `json:"scheme" binding:"required"`
	Address string `json:"address" binding:"required"`
}

type issueNonceResponse struct {
	Nonce     string    `json:"nonce"`
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
	c.JSON(http.StatusCreated, issueNonceResponse{Nonce: nonce.Value, ExpiresAt: nonce.ExpiresAt})
}
