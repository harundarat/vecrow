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
	LinkWallet(ctx context.Context, accountID string, p auth.Proof) (domain.Account, bool, error)
	UnlinkWallet(ctx context.Context, accountID, walletID string) error
}

// MeHandler serves the signed-in account's endpoints. Every route runs
// behind middleware.RequireSession.
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
	ID        string           `json:"id"`
	Username  string           `json:"username"`
	CreatedAt time.Time        `json:"created_at"`
	Wallets   []walletResponse `json:"wallets"`
}

func newAccountResponse(a domain.Account) accountResponse {
	res := accountResponse{
		ID:        a.ID,
		Username:  a.Username,
		CreatedAt: a.CreatedAt,
		Wallets:   make([]walletResponse, len(a.Wallets)),
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

// accountID returns the signed-in account, or writes a 401 and returns false.
func accountID(c *gin.Context) (string, bool) {
	sess, ok := middleware.SessionFrom(c.Request.Context())
	if !ok {
		// Only reachable if a route is registered without RequireSession.
		response.Error(c, errors.New("handler: route requires middleware.RequireSession"))
		return "", false
	}
	return sess.AccountID, true
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

type updateMeRequest struct {
	Username string `json:"username" binding:"required"`
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
	account, err := h.accounts.ChangeUsername(c.Request.Context(), id, req.Username)
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, newAccountResponse(account))
}

// LinkWallet answers 201 when the wallet was linked and 200 when it already
// belonged to this account.
func (h *MeHandler) LinkWallet(c *gin.Context) {
	id, ok := accountID(c)
	if !ok {
		return
	}
	var req proofRequest
	if !bindJSON(c, &req) {
		return
	}
	account, linked, err := h.accounts.LinkWallet(c.Request.Context(), id, req.proof())
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
