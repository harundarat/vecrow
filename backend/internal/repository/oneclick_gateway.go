package repository

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/harundarat/vecrow/backend/internal/domain"
	"github.com/harundarat/vecrow/backend/internal/infrastructure/oneclick"
	"github.com/harundarat/vecrow/backend/internal/usecase/funding"
)

// OneClickGateway adapts the 1Click client to the funding use case: the
// escrow is funded with an EXACT_OUTPUT swap from the origin chain to USDC
// on NEAR, delivered to the contract (R-08).
type OneClickGateway struct {
	client *oneclick.Client
}

var _ funding.OneClick = (*OneClickGateway)(nil)

func NewOneClickGateway(c *oneclick.Client) *OneClickGateway {
	return &OneClickGateway{client: c}
}

func (g *OneClickGateway) Tokens(ctx context.Context) ([]domain.Token, error) {
	tokens, err := g.client.Tokens(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Token, len(tokens))
	for i, t := range tokens {
		out[i] = domain.Token{
			AssetID: t.AssetID, Symbol: t.Symbol, Blockchain: t.Blockchain, Decimals: t.Decimals,
			Price: float64(t.Price), ContractAddress: t.ContractAddress,
		}
	}
	return out, nil
}

func (g *OneClickGateway) Quote(ctx context.Context, r funding.QuoteRequest) (funding.Quote, error) {
	res, err := g.client.Quote(ctx, oneclick.QuoteRequest{
		SwapType:           oneclick.SwapExactOutput,
		SlippageTolerance:  r.SlippageBPS,
		OriginAsset:        r.OriginAsset,
		DepositType:        oneclick.DepositOriginChain,
		DestinationAsset:   r.DestinationAsset,
		Amount:             r.Amount,
		RefundTo:           r.RefundTo,
		RefundType:         oneclick.RefundOriginChain,
		Recipient:          r.Recipient,
		RecipientType:      oneclick.RecipientDestinationChain,
		Deadline:           r.Deadline,
		CustomRecipientMsg: r.CustomRecipientMsg,
	})
	var apiErr *oneclick.Error
	if errors.As(err, &apiErr) && (apiErr.StatusCode == http.StatusBadRequest || apiErr.StatusCode == http.StatusNotFound) {
		msg := apiErr.Message
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d", apiErr.StatusCode)
		}
		return funding.Quote{}, &funding.QuoteRejectedError{Message: msg}
	}
	if err != nil {
		return funding.Quote{}, err
	}
	q := res.Quote
	return funding.Quote{
		DepositAddress:    q.DepositAddress,
		DepositMemo:       q.DepositMemo,
		AmountIn:          q.AmountIn,
		AmountInFormatted: q.AmountInFormatted,
		AmountOut:         q.AmountOut,
		Deadline:          q.Deadline,
		Raw:               res.Raw,
	}, nil
}

func (g *OneClickGateway) Status(ctx context.Context, depositAddress, depositMemo string) (string, error) {
	st, err := g.client.Status(ctx, depositAddress, depositMemo)
	if errors.Is(err, oneclick.ErrNotFound) {
		return "", funding.ErrDepositNotFound
	}
	if err != nil {
		return "", err
	}
	return st.Status, nil
}
