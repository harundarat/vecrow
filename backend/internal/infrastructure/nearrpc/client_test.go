package nearrpc_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/harundarat/vecrow/backend/internal/infrastructure/nearrpc"
)

// Response bodies captured from rpc.testnet.near.org (block data trimmed).
const (
	fullAccessBody   = `{"jsonrpc":"2.0","result":{"block_hash":"h","block_height":1,"nonce":5,"permission":"FullAccess"},"id":"vecrow"}`
	functionCallBody = `{"jsonrpc":"2.0","result":{"block_hash":"h","block_height":1,"nonce":5,"permission":{"FunctionCall":{"allowance":"250000000000000000000000","receiver_id":"app.testnet","method_names":[]}}},"id":"vecrow"}`
	// view_access_key reports a missing key inside result, also for accounts
	// that do not exist.
	unknownKeyBody = `{"jsonrpc":"2.0","result":{"block_hash":"h","block_height":1,"error":"access key ed25519:9hSR6S7WPtxmTojgo6GG3k4yDPecgJY292j7xrsUGWBu does not exist while viewing","logs":[]},"id":"vecrow"}`
	// view_account reports a missing account as an RPC error.
	unknownAccountBody = `{"jsonrpc":"2.0","error":{"name":"HANDLER_ERROR","cause":{"info":{"requested_account_id":"x"},"name":"UNKNOWN_ACCOUNT"},"code":-32000,"message":"Server error","data":"account x does not exist while viewing"},"id":"vecrow"}`
	accountBody        = `{"jsonrpc":"2.0","result":{"amount":"0","locked":"0","code_hash":"11111111111111111111111111111111","storage_usage":182,"block_height":1,"block_hash":"h"},"id":"vecrow"}`
	internalErrorBody  = `{"jsonrpc":"2.0","error":{"name":"INTERNAL_ERROR","cause":{"name":"INTERNAL_ERROR"},"code":-32000,"message":"Server error","data":"oops"},"id":"vecrow"}`
)

// server answers each request with the body for its request_type and
// records the decoded requests.
func server(t *testing.T, status int, bodies map[string]string) (*httptest.Server, *[]map[string]any) {
	t.Helper()
	var requests []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var req map[string]any
		if err := json.Unmarshal(b, &req); err != nil {
			t.Errorf("request is not JSON: %s", b)
		}
		requests = append(requests, req)
		params, _ := req["params"].(map[string]any)
		kind, _ := params["request_type"].(string)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, bodies[kind])
	}))
	t.Cleanup(srv.Close)
	return srv, &requests
}

func TestViewAccessKey(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    nearrpc.Permission
		wantErr error
	}{
		{name: "full access", body: fullAccessBody, want: nearrpc.FullAccess},
		{name: "function call", body: functionCallBody, want: nearrpc.FunctionCall},
		{name: "unknown key", body: unknownKeyBody, wantErr: nearrpc.ErrUnknownAccessKey},
		{name: "unknown account as rpc error", body: unknownAccountBody, wantErr: nearrpc.ErrUnknownAccount},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, requests := server(t, http.StatusOK, map[string]string{"view_access_key": tt.body})
			got, err := nearrpc.New(srv.URL, time.Second).ViewAccessKey(context.Background(), "alice.testnet", "ed25519:abc")
			if !errors.Is(err, tt.wantErr) || got != tt.want {
				t.Fatalf("ViewAccessKey = %q, %v; want %q, %v", got, err, tt.want, tt.wantErr)
			}
			req := (*requests)[0]
			params := req["params"].(map[string]any)
			if req["method"] != "query" || params["finality"] != "final" ||
				params["account_id"] != "alice.testnet" || params["public_key"] != "ed25519:abc" {
				t.Errorf("request = %v", req)
			}
		})
	}
}

func TestAccountExists(t *testing.T) {
	srv, _ := server(t, http.StatusOK, map[string]string{"view_account": accountBody})
	if ok, err := nearrpc.New(srv.URL, time.Second).AccountExists(context.Background(), "alice.testnet"); err != nil || !ok {
		t.Fatalf("existing account: %v, %v", ok, err)
	}
	srv, _ = server(t, http.StatusOK, map[string]string{"view_account": unknownAccountBody})
	if ok, err := nearrpc.New(srv.URL, time.Second).AccountExists(context.Background(), "x"); err != nil || ok {
		t.Fatalf("missing account: %v, %v", ok, err)
	}
}

func TestErrors(t *testing.T) {
	t.Run("rpc error", func(t *testing.T) {
		srv, _ := server(t, http.StatusOK, map[string]string{"view_access_key": internalErrorBody})
		_, err := nearrpc.New(srv.URL, time.Second).ViewAccessKey(context.Background(), "a.testnet", "ed25519:x")
		if err == nil || errors.Is(err, nearrpc.ErrUnknownAccessKey) || !strings.Contains(err.Error(), "INTERNAL_ERROR") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("non-JSON response", func(t *testing.T) {
		srv, _ := server(t, http.StatusBadGateway, map[string]string{"view_access_key": "<html>bad gateway</html>"})
		_, err := nearrpc.New(srv.URL, time.Second).ViewAccessKey(context.Background(), "a.testnet", "ed25519:x")
		if err == nil || !strings.Contains(err.Error(), "HTTP 502") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("connection error hides the URL", func(t *testing.T) {
		srv, _ := server(t, http.StatusOK, nil)
		url := srv.URL + "/secret-api-key"
		srv.Close()
		_, err := nearrpc.New(url, time.Second).ViewAccessKey(context.Background(), "a.testnet", "ed25519:x")
		if err == nil || strings.Contains(err.Error(), "secret-api-key") {
			t.Fatalf("error = %v, want a connection error without the URL", err)
		}
	})
}
