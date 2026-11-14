package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/harundarat/vecrow/backend/internal/delivery/http/middleware"
	"github.com/harundarat/vecrow/backend/internal/delivery/http/response"
	"github.com/harundarat/vecrow/backend/internal/domain"
	"github.com/harundarat/vecrow/backend/internal/usecase/funding"
)

// FundingUseCase is the use case behind GET /tokens and the funding
// endpoints.
type FundingUseCase interface {
	Tokens(ctx context.Context) ([]domain.Token, error)
	RequestQuote(ctx context.Context, id string, req domain.Requester, in funding.QuoteInput) (domain.FundingQuote, error)
	Status(ctx context.Context, id string, req domain.Requester) (funding.View, error)
}

// FundingHandler serves 1Click funding (R-08).
type FundingHandler struct {
	funding FundingUseCase
}

func NewFundingHandler(f FundingUseCase) *FundingHandler {
	return &FundingHandler{funding: f}
}

type tokenJSON struct {
	AssetID         string  `json:"asset_id"`
	Symbol          string  `json:"symbol"`
	Blockchain      string  `json:"blockchain"`
	Decimals        int     `json:"decimals"`
	Price           float64 `json:"price"`
	ContractAddress string  `json:"contract_address,omitempty"`
}

type tokensResponse struct {
	Tokens []tokenJSON `json:"tokens"`
}

// Tokens lists the tokens 1Click can fund an escrow from.
func (h *FundingHandler) Tokens(c *gin.Context) {
	tokens, err := h.funding.Tokens(c.Request.Context())
	if err != nil {
		response.Error(c, err)
		return
	}
	out := tokensResponse{Tokens: make([]tokenJSON, len(tokens))}
	for i, t := range tokens {
		out.Tokens[i] = tokenJSON{AssetID: t.AssetID, Symbol: t.Symbol, Blockchain: t.Blockchain,
			Decimals: t.Decimals, Price: t.Price, ContractAddress: t.ContractAddress}
	}
	c.JSON(http.StatusOK, out)
}

type quoteRequest struct {
	OriginAsset string `json:"origin_asset" binding:"required"`
	RefundTo    string `json:"refund_to" binding:"required"`
}

type quoteJSON struct {
	ID                string     `json:"id"`
	OriginAsset       string     `json:"origin_asset"`
	RefundTo          string     `json:"refund_to"`
	DepositAddress    string     `json:"deposit_address"`
	DepositMemo       string     `json:"deposit_memo,omitempty"`
	AmountIn          string     `json:"amount_in"`
	AmountInFormatted string     `json:"amount_in_formatted"`
	AmountOut         string     `json:"amount_out"`
	Deadline          *time.Time `json:"deadline"`
	Status            string     `json:"status"`
	OneClickStatus    string     `json:"oneclick_status,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
}

func newQuoteJSON(q domain.FundingQuote) quoteJSON {
	return quoteJSON{
		ID: q.ID, OriginAsset: q.OriginAsset, RefundTo: q.RefundTo, DepositAddress: q.DepositAddress,
		DepositMemo: q.DepositMemo, AmountIn: q.AmountIn, AmountInFormatted: q.AmountInFormatted, AmountOut: q.AmountOut,
		Deadline: q.Deadline, Status: string(q.Status), OneClickStatus: q.OneClickStatus, CreatedAt: q.CreatedAt,
	}
}

// Quote returns a 1Click deposit address for the approver (R-08).
func (h *FundingHandler) Quote(c *gin.Context) {
	var req quoteRequest
	if !bindJSON(c, &req) {
		return
	}
	q, err := h.funding.RequestQuote(c.Request.Context(), c.Param("id"), middleware.RequesterFrom(c.Request.Context()),
		funding.QuoteInput{OriginAsset: req.OriginAsset, RefundTo: req.RefundTo})
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, newQuoteJSON(q))
}

type fundingResponse struct {
	Status         string    `json:"status"`
	OneClickStatus string    `json:"oneclick_status,omitempty"`
	EscrowStatus   string    `json:"escrow_status"`
	Quote          quoteJSON `json:"quote"`
}

// Status returns the funding status: PENDING, SUCCESS or REFUNDED.
func (h *FundingHandler) Status(c *gin.Context) {
	v, err := h.funding.Status(c.Request.Context(), c.Param("id"), middleware.RequesterFrom(c.Request.Context()))
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, fundingResponse{
		Status: string(v.Status), OneClickStatus: v.Quote.OneClickStatus, EscrowStatus: string(v.EscrowStatus),
		Quote: newQuoteJSON(v.Quote),
	})
}
