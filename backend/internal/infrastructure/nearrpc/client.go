// Package nearrpc is a minimal NEAR JSON-RPC client for the view queries the
// backend needs.
package nearrpc

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	// ErrUnknownAccount means the account does not exist on chain.
	ErrUnknownAccount = errors.New("nearrpc: unknown account")
	// ErrUnknownAccessKey means the access key is not on the account. NEAR
	// also reports this when the account itself does not exist.
	ErrUnknownAccessKey = errors.New("nearrpc: unknown access key")
)

// Permission is an access key's permission.
type Permission string

const (
	FullAccess   Permission = "FullAccess"
	FunctionCall Permission = "FunctionCall"
)

const maxResponseBytes = 1 << 20

// Client calls one RPC endpoint.
type Client struct {
	url  string
	http *http.Client
}

// New returns a client for rpcURL; each call is bounded by timeout.
func New(rpcURL string, timeout time.Duration) *Client {
	return &Client{url: rpcURL, http: &http.Client{Timeout: timeout}}
}

// ViewAccessKey returns the permission of publicKey ("ed25519:...") on the
// account at the latest final block.
func (c *Client) ViewAccessKey(ctx context.Context, accountID, publicKey string) (Permission, error) {
	var res struct {
		Permission json.RawMessage `json:"permission"`
		// Some nodes report errors inside result instead of as an RPC error.
		Error string `json:"error"`
	}
	err := c.query(ctx, map[string]string{
		"request_type": "view_access_key",
		"finality":     "final",
		"account_id":   accountID,
		"public_key":   publicKey,
	}, &res)
	if err != nil {
		return "", err
	}
	if res.Error != "" {
		if strings.Contains(res.Error, "does not exist") {
			if strings.HasPrefix(res.Error, "access key") {
				return "", ErrUnknownAccessKey
			}
			return "", ErrUnknownAccount
		}
		return "", fmt.Errorf("nearrpc: view_access_key: %s", res.Error)
	}

	// "FullAccess" or {"FunctionCall": {...}}.
	var full string
	if json.Unmarshal(res.Permission, &full) == nil && full == string(FullAccess) {
		return FullAccess, nil
	}
	var limited map[string]json.RawMessage
	if json.Unmarshal(res.Permission, &limited) == nil {
		if _, ok := limited[string(FunctionCall)]; ok {
			return FunctionCall, nil
		}
	}
	return "", fmt.Errorf("nearrpc: view_access_key: unexpected permission %.100s", res.Permission)
}

// AccountExists reports whether the account exists at the latest final block.
func (c *Client) AccountExists(ctx context.Context, accountID string) (bool, error) {
	var res struct {
		Error string `json:"error"`
	}
	err := c.query(ctx, map[string]string{
		"request_type": "view_account",
		"finality":     "final",
		"account_id":   accountID,
	}, &res)
	if errors.Is(err, ErrUnknownAccount) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if res.Error != "" {
		if strings.Contains(res.Error, "does not exist") {
			return false, nil
		}
		return false, fmt.Errorf("nearrpc: view_account: %s", res.Error)
	}
	return true, nil
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      string `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

type rpcResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

type rpcError struct {
	Name  string `json:"name"`
	Cause struct {
		Name string `json:"name"`
	} `json:"cause"`
	Message string `json:"message"`
}

// query calls the "query" method and decodes the result into out.
func (c *Client) query(ctx context.Context, params map[string]string, out any) error {
	kind := params["request_type"]
	body, err := json.Marshal(rpcRequest{JSONRPC: "2.0", ID: "vecrow", Method: "query", Params: params})
	if err != nil {
		return fmt.Errorf("nearrpc: %s: encode request: %w", kind, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("nearrpc: %s: %w", kind, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		// RPC URLs of hosted providers can carry an API key, so the URL that
		// *url.Error includes is left out.
		var uerr *url.Error
		if errors.As(err, &uerr) {
			err = uerr.Err
		}
		return fmt.Errorf("nearrpc: %s: request failed: %w", kind, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return fmt.Errorf("nearrpc: %s: read response: %w", kind, err)
	}
	var r rpcResponse
	if err := json.Unmarshal(raw, &r); err != nil {
		return fmt.Errorf("nearrpc: %s: unexpected response (HTTP %d)", kind, resp.StatusCode)
	}
	if r.Error != nil {
		switch r.Error.Cause.Name {
		case "UNKNOWN_ACCOUNT":
			return ErrUnknownAccount
		case "UNKNOWN_ACCESS_KEY":
			return ErrUnknownAccessKey
		}
		return fmt.Errorf("nearrpc: %s: %s (%s)", kind, r.Error.Message, cmp.Or(r.Error.Cause.Name, r.Error.Name))
	}
	if resp.StatusCode != http.StatusOK || len(r.Result) == 0 {
		return fmt.Errorf("nearrpc: %s: unexpected response (HTTP %d)", kind, resp.StatusCode)
	}
	if err := json.Unmarshal(r.Result, out); err != nil {
		return fmt.Errorf("nearrpc: %s: decode result: %w", kind, err)
	}
	return nil
}
