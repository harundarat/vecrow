package handler

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/harundarat/vecrow/backend/internal/delivery/http/middleware"
	"github.com/harundarat/vecrow/backend/internal/delivery/http/response"
	"github.com/harundarat/vecrow/backend/internal/domain"
	"github.com/harundarat/vecrow/backend/internal/usecase/auth"
)

// AccountUseCase is the use case behind the /me endpoints.
type AccountUseCase interface {
	Get(ctx context.Context, accountID string) (domain.Account, error)
	ChangeUsername(ctx context.Context, accountID, username string) (domain.Account, error)
	ChangeEmail(ctx context.Context, accountID, email string) (domain.Account, error)
	VerifyEmail(ctx context.Context, accountID, code string) (domain.Account, error)
	LinkWallet(ctx context.Context, sess domain.Session, p auth.Proof) (domain.Account, bool, error)
	UnlinkWallet(ctx context.Context, accountID, walletID string) error
}

// MeHandler serves the signed-in account's endpoints. Every route runs
// behind middleware.RequireSession or, for the routes a recovery session may
// use, middleware.RequireAnySession.
type MeHandler struct {
	accounts AccountUseCase
}

func NewMeHandler(accounts AccountUseCase) *MeHandler {
	return &MeHandler{accounts: accounts}
}

type walletResponse struct {
	ID        string    `json:"id"`
	Scheme    string    `json:"scheme"`
	Address   string    `json:"address"`
	CreatedAt time.Time `json:"created_at"`
}

type accountResponse struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	// Email is the verified email, or null.
	Email *string `json:"email"`
	// PendingEmail is a new email waiting for its code, or null.
	PendingEmail *string          `json:"pending_email"`
	CreatedAt    time.Time        `json:"created_at"`
	Wallets      []walletResponse `json:"wallets"`
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func newAccountResponse(a domain.Account) accountResponse {
	res := accountResponse{
		ID:           a.ID,
		Username:     a.Username,
		Email:        optional(a.Email),
		PendingEmail: optional(a.PendingEmail),
		CreatedAt:    a.CreatedAt,
		Wallets:      make([]walletResponse, len(a.Wallets)),
	}
	for i, w := range a.Wallets {
		res.Wallets[i] = walletResponse{
			ID:        w.ID,
			Scheme:    string(w.Wallet.Scheme),
			Address:   w.Wallet.Address,
			CreatedAt: w.CreatedAt,
		}
	}
	return res
}

// accountID returns the signed-in account, or writes a 500 and returns false.
func accountID(c *gin.Context) (string, bool) {
	sess, ok := session(c)
	return sess.AccountID, ok
}

// session returns the signed-in session, or writes a 500 and returns false.
func session(c *gin.Context) (domain.Session, bool) {
	sess, ok := middleware.SessionFrom(c.Request.Context())
	if !ok {
		// Only reachable if a route is registered without RequireSession.
		response.Error(c, errors.New("handler: route requires middleware.RequireSession"))
		return domain.Session{}, false
	}
	return sess, true
}

func (h *MeHandler) Get(c *gin.Context) {
	id, ok := accountID(c)
	if !ok {
		return
	}
	account, err := h.accounts.Get(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, newAccountResponse(account))
}

// updateMeRequest changes the username, the email, or both. An email starts
// as pending and is verified with POST /me/email/verify; "" removes it.
type updateMeRequest struct {
	Username *string `json:"username"`
	Email    *string `json:"email"`
}

func (h *MeHandler) Update(c *gin.Context) {
	id, ok := accountID(c)
	if !ok {
		return
	}
	var req updateMeRequest
	if !bindJSON(c, &req) {
		return
	}
	if req.Username == nil && req.Email == nil {
		response.Error(c, domain.InvalidArgument("username or email is required"))
		return
	}
	ctx := c.Request.Context()
	var account domain.Account
	var err error
	if req.Username != nil {
		if account, err = h.accounts.ChangeUsername(ctx, id, *req.Username); err != nil {
			response.Error(c, err)
			return
		}
	}
	if req.Email != nil {
		if account, err = h.accounts.ChangeEmail(ctx, id, *req.Email); err != nil {
			response.Error(c, err)
			return
		}
	}
	c.JSON(http.StatusOK, newAccountResponse(account))
}

type verifyEmailRequest struct {
	Code string `json:"code" binding:"required"`
}

// VerifyEmail checks the code sent to the pending email (R-14).
func (h *MeHandler) VerifyEmail(c *gin.Context) {
	id, ok := accountID(c)
	if !ok {
		return
	}
	var req verifyEmailRequest
	if !bindJSON(c, &req) {
		return
	}
	account, err := h.accounts.VerifyEmail(c.Request.Context(), id, req.Code)
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, newAccountResponse(account))
}

// LinkWallet answers 201 when the wallet was linked and 200 when it already
// belonged to this account.
func (h *MeHandler) LinkWallet(c *gin.Context) {
	sess, ok := session(c)
	if !ok {
		return
	}
	var req proofRequest
	if !bindJSON(c, &req) {
		return
	}
	account, linked, err := h.accounts.LinkWallet(c.Request.Context(), sess, req.proof())
	if err != nil {
		response.Error(c, err)
		return
	}
	status := http.StatusOK
	if linked {
		status = http.StatusCreated
	}
	c.JSON(status, newAccountResponse(account))
}

func (h *MeHandler) UnlinkWallet(c *gin.Context) {
	id, ok := accountID(c)
	if !ok {
		return
	}
	if err := h.accounts.UnlinkWallet(c.Request.Context(), id, c.Param("id")); err != nil {
		response.Error(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
