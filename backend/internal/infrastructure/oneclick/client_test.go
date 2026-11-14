package oneclick_test

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
)

// Responses shaped like the examples of the 1Click OpenAPI document.
const (
	tokensResponse = `[
	  {"assetId":"nep141:wrap.near","decimals":24,"blockchain":"near","symbol":"wNEAR","price":"2.79","priceUpdatedAt":"2025-03-28T12:23:00.070Z","contractAddress":"wrap.near"},
	  {"assetId":"nep141:eth.omft.near","decimals":18,"blockchain":"eth","symbol":"ETH","price":3500.12,"priceUpdatedAt":"2025-03-28T12:23:00.070Z"}
	]`
	quoteResponse = `{
	  "correlationId":"ca6ee8f0-a4e1-4e5a-a2a3-3b5c1d2e3f40","timestamp":"2026-11-12T10:00:00.000Z","signature":"ed25519:sig",
	  "quoteRequest":{"dry":false,"swapType":"EXACT_OUTPUT"},
	  "quote":{"depositAddress":"0x76b4c56085ED136a8744D52bE956396624a730E8","amountIn":"1000000000000000","amountInFormatted":"0.001",
	    "amountInUsd":"3.50","minAmountIn":"995000000000000","amountOut":"1500000","amountOutFormatted":"1.5","amountOutUsd":"1.50",
	    "minAmountOut":"1500000","timeEstimate":120,"deadline":"2026-11-12T11:00:00.000Z","timeWhenInactive":"2026-11-12T11:00:00.000Z"}
	}`
	statusResponse = `{
	  "correlationId":"ca6ee8f0","quoteResponse":{},"status":"SUCCESS","updatedAt":"2026-11-12T10:05:00.000Z",
	  "swapDetails":{"intentHashes":["h"],"nearTxHashes":["NT1","NT2"],"amountIn":"1000000000000000","amountOut":"1500000",
	    "originChainTxHashes":[{"hash":"0xabc","explorerUrl":"https://etherscan.io/tx/0xabc"}],"destinationChainTxHashes":[]}
	}`
)

func TestTokens(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v0/tokens" {
			t.Errorf("request %s %s", r.Method, r.URL)
		}
		_, _ = io.WriteString(w, tokensResponse)
	}))
	defer srv.Close()
	tokens, err := oneclick.New(srv.URL, "", srv.Client()).Tokens(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tokens) != 2 || tokens[0].AssetID != "nep141:wrap.near" || tokens[0].Price != 2.79 || tokens[0].Decimals != 24 ||
		tokens[1].Price != 3500.12 || tokens[1].Blockchain != "eth" || tokens[0].ContractAddress != "wrap.near" {
		t.Fatalf("tokens = %+v", tokens)
	}
}

func TestQuote(t *testing.T) {
	deadline := time.Date(2026, 11, 12, 11, 0, 0, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v0/quote" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("request %s %s", r.Method, r.URL)
		}
		if r.Header.Get("Authorization") != "Bearer JWT" {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		want := map[string]any{
			"dry": false, "swapType": "EXACT_OUTPUT", "slippageTolerance": float64(100),
			"originAsset": "nep141:eth.omft.near", "depositType": "ORIGIN_CHAIN",
			"destinationAsset": "nep141:usdc.near", "amount": "1500000",
			"refundTo": "0x7e5f4552091a69125d5dfcb7b8c2659029395bdf", "refundType": "ORIGIN_CHAIN",
			"recipient": "escrow.near", "recipientType": "DESTINATION_CHAIN",
			"deadline": "2026-11-12T11:00:00Z", "customRecipientMsg": "escrow-1",
		}
		for k, v := range want {
			if body[k] != v {
				t.Errorf("%s = %#v, want %#v", k, body[k], v)
			}
		}
		_, _ = io.WriteString(w, quoteResponse)
	}))
	defer srv.Close()

	res, err := oneclick.New(srv.URL, "JWT", srv.Client()).Quote(context.Background(), oneclick.QuoteRequest{
		SwapType: oneclick.SwapExactOutput, SlippageTolerance: 100, OriginAsset: "nep141:eth.omft.near",
		DepositType: oneclick.DepositOriginChain, DestinationAsset: "nep141:usdc.near", Amount: "1500000",
		RefundTo: "0x7e5f4552091a69125d5dfcb7b8c2659029395bdf", RefundType: oneclick.RefundOriginChain,
		Recipient: "escrow.near", RecipientType: oneclick.RecipientDestinationChain, Deadline: deadline,
		CustomRecipientMsg: "escrow-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	q := res.Quote
	if q.DepositAddress != "0x76b4c56085ED136a8744D52bE956396624a730E8" || q.AmountIn != "1000000000000000" ||
		q.AmountInFormatted != "0.001" || q.AmountOut != "1500000" || q.TimeEstimate != 120 || !q.Deadline.Equal(deadline) {
		t.Fatalf("quote = %+v", q)
	}
	if string(res.Raw) != quoteResponse {
		t.Fatal("raw response not kept")
	}
}

func TestQuoteRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"message":"Amount is too low for bridge, try at least 1000000"}`)
	}))
	defer srv.Close()
	_, err := oneclick.New(srv.URL, "", srv.Client()).Quote(context.Background(), oneclick.QuoteRequest{})
	var apiErr *oneclick.Error
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 400 || apiErr.Message != "Amount is too low for bridge, try at least 1000000" {
		t.Fatalf("error = %v", err)
	}
}

func TestStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v0/status" {
			t.Errorf("path %s", r.URL.Path)
		}
		switch r.URL.Query().Get("depositAddress") {
		case "0xknown":
			if r.URL.Query().Get("depositMemo") != "memo1" {
				t.Errorf("memo = %q", r.URL.Query().Get("depositMemo"))
			}
			_, _ = io.WriteString(w, statusResponse)
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"message":"Deposit address not found"}`)
		}
	}))
	defer srv.Close()
	c := oneclick.New(srv.URL, "", srv.Client())
	st, err := c.Status(context.Background(), "0xknown", "memo1")
	if err != nil || st.Status != oneclick.StatusSuccess || len(st.SwapDetails.NearTxHashes) != 2 || st.SwapDetails.AmountOut != "1500000" {
		t.Fatalf("status = %+v, %v", st, err)
	}
	if _, err := c.Status(context.Background(), "0xunknown", ""); !errors.Is(err, oneclick.ErrNotFound) {
		t.Fatalf("unknown address: %v", err)
	}
}

func TestUnauthorizedWithoutBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	_, err := oneclick.New(srv.URL, "bad", srv.Client()).Status(context.Background(), "0x1", "")
	var apiErr *oneclick.Error
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 401 || errors.Is(err, oneclick.ErrNotFound) {
		t.Fatalf("error = %v", err)
	}
}
