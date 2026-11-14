//go:build e2e

package e2e_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

const oneClickKey = "e2e-oneclick-key"

// fakeOneClick serves the 1Click endpoints the backend uses, shaped like the
// documented API (https://docs.near-intents.org/integration/distribution-channels/1click-api).
// The test plays the solver: it delivers the USDC itself, then sets the
// swap's status.
type fakeOneClick struct {
	t   *testing.T
	srv *httptest.Server

	mu       sync.Mutex
	quotes   map[string]map[string]any // deposit address → quote request
	statuses map[string]string         // deposit address → status
	polls    map[string]int            // deposit address → status calls
}

func newFakeOneClick(t *testing.T, usdcContract string) *fakeOneClick {
	f := &fakeOneClick{t: t, quotes: map[string]map[string]any{}, statuses: map[string]string{}, polls: map[string]int{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v0/tokens", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, []map[string]any{
			{"assetId": "nep141:" + usdcContract, "decimals": 6, "blockchain": "near", "symbol": "USDC",
				"price": 1, "priceUpdatedAt": time.Now().UTC(), "contractAddress": usdcContract},
			{"assetId": "nep141:eth.omft.near", "decimals": 18, "blockchain": "eth", "symbol": "ETH",
				"price": "3000.12", "priceUpdatedAt": time.Now().UTC()},
		})
	})
	mux.HandleFunc("POST /v0/quote", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+oneClickKey {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "Unauthorized"})
			return
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
			return
		}
		f.mu.Lock()
		addr := fmt.Sprintf("0x%040x", len(f.quotes)+1)
		f.quotes[addr] = req
		f.statuses[addr] = "PENDING_DEPOSIT"
		f.mu.Unlock()
		writeJSON(w, http.StatusCreated, map[string]any{
			"timestamp":     time.Now().UTC(),
			"signature":     "ed25519:fake",
			"correlationId": "corr-" + addr,
			"quoteRequest":  req,
			"quote": map[string]any{
				"depositAddress":     addr,
				"amountIn":           "1000000000000000000",
				"amountInFormatted":  "1.0",
				"amountInUsd":        "3000.12",
				"minAmountIn":        "1000000000000000000",
				"amountOut":          req["amount"],
				"amountOutFormatted": "x",
				"amountOutUsd":       "x",
				"minAmountOut":       req["amount"],
				"timeEstimate":       120,
				"deadline":           req["deadline"],
			},
		})
	})
	mux.HandleFunc("GET /v0/status", func(w http.ResponseWriter, r *http.Request) {
		addr := r.URL.Query().Get("depositAddress")
		f.mu.Lock()
		status, ok := f.statuses[addr]
		f.polls[addr]++
		f.mu.Unlock()
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"message": "Deposit address not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"correlationId": "corr-" + addr,
			"status":        status,
			"updatedAt":     time.Now().UTC(),
			"swapDetails":   map[string]any{"nearTxHashes": []string{}, "amountOut": "0"},
		})
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (f *fakeOneClick) quote(addr string) map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.quotes[addr]
}

func (f *fakeOneClick) setStatus(addr, status string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statuses[addr] = status
}

func (f *fakeOneClick) pollCount(addr string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.polls[addr]
}
