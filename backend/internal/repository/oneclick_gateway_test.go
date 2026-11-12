package repository_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/harundarat/vecrow/backend/internal/infrastructure/oneclick"
	"github.com/harundarat/vecrow/backend/internal/repository"
	"github.com/harundarat/vecrow/backend/internal/usecase/funding"
)

func TestOneClickGateway(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v0/tokens":
			_, _ = io.WriteString(w, `[{"assetId":"nep141:wrap.near","decimals":24,"blockchain":"near","symbol":"wNEAR","price":2.79}]`)
		case "/v0/quote":
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["originAsset"] == "nep141:bad" {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = io.WriteString(w, `{"message":"originAsset is not supported"}`)
				return
			}
			_, _ = io.WriteString(w, `{"correlationId":"c","quote":{"depositAddress":"0xdep","amountIn":"10","amountInFormatted":"0.00001","amountOut":"1500000","timeEstimate":60}}`)
		case "/v0/status":
			if r.URL.Query().Get("depositAddress") != "0xdep" {
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, `{"message":"Deposit address not found"}`)
				return
			}
			_, _ = io.WriteString(w, `{"status":"PROCESSING","updatedAt":"2026-11-12T10:00:00Z","swapDetails":{}}`)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()
	g := repository.NewOneClickGateway(oneclick.New(srv.URL, "", srv.Client()))
	ctx := context.Background()

	tokens, err := g.Tokens(ctx)
	if err != nil || len(tokens) != 1 || tokens[0].Symbol != "wNEAR" || tokens[0].Price != 2.79 {
		t.Fatalf("Tokens = %+v, %v", tokens, err)
	}

	req := funding.QuoteRequest{
		OriginAsset: "nep141:eth.omft.near", DestinationAsset: "nep141:usdc.near", Amount: "1500000",
		RefundTo: "0xabc", Recipient: "escrow.near", CustomRecipientMsg: "escrow-1", SlippageBPS: 100,
		Deadline: time.Date(2026, 11, 12, 11, 0, 0, 0, time.UTC),
	}
	q, err := g.Quote(ctx, req)
	if err != nil || q.DepositAddress != "0xdep" || q.AmountOut != "1500000" || len(q.Raw) == 0 {
		t.Fatalf("Quote = %+v, %v", q, err)
	}
	for k, v := range map[string]any{
		"swapType": "EXACT_OUTPUT", "recipient": "escrow.near", "recipientType": "DESTINATION_CHAIN",
		"customRecipientMsg": "escrow-1", "destinationAsset": "nep141:usdc.near", "depositType": "ORIGIN_CHAIN",
		"refundType": "ORIGIN_CHAIN", "dry": false, "amount": "1500000",
	} {
		if body[k] != v {
			t.Errorf("quote %s = %#v, want %#v", k, body[k], v)
		}
	}
	req.OriginAsset = "nep141:bad"
	var rejected *funding.QuoteRejectedError
	if _, err := g.Quote(ctx, req); !errors.As(err, &rejected) || rejected.Message != "originAsset is not supported" {
		t.Fatalf("refused quote: %v", err)
	}

	if st, err := g.Status(ctx, "0xdep", ""); err != nil || st != "PROCESSING" {
		t.Fatalf("Status = %q, %v", st, err)
	}
	if _, err := g.Status(ctx, "0xother", ""); !errors.Is(err, funding.ErrDepositNotFound) {
		t.Fatalf("unknown deposit: %v", err)
	}
}
