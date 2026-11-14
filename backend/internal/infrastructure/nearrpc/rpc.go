package nearrpc

import (
	"bytes"
	"cmp"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Error is an error returned by the RPC node.
type Error struct {
	Method string
	// Name and Cause classify it, e.g. HANDLER_ERROR / INVALID_TRANSACTION.
	Name  string
	Cause string
	// Message and Data describe it; Data holds the structured error, e.g. a
	// TxExecutionError.
	Message string
	Data    json.RawMessage
}

func (e *Error) Error() string {
	return fmt.Sprintf("nearrpc: %s: %s (%s)%s", e.Method, e.Message, cmp.Or(e.Cause, e.Name), dataSuffix(e.Data))
}

func dataSuffix(data json.RawMessage) string {
	if len(data) == 0 {
		return ""
	}
	s := string(data)
	if len(s) > 300 {
		s = s[:300] + "…"
	}
	return ": " + s
}

type rpcError struct {
	Name  string `json:"name"`
	Cause struct {
		Name string          `json:"name"`
		Info json.RawMessage `json:"info"`
	} `json:"cause"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// call sends one JSON-RPC request and decodes its result into out.
func (c *Client) call(ctx context.Context, method string, params any, out any) error {
	body, err := json.Marshal(rpcRequest{JSONRPC: "2.0", ID: "vecrow", Method: method, Params: params})
	if err != nil {
		return fmt.Errorf("nearrpc: %s: encode request: %w", method, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("nearrpc: %s: %w", method, err)
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
		return fmt.Errorf("nearrpc: %s: request failed: %w", method, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return fmt.Errorf("nearrpc: %s: read response: %w", method, err)
	}
	var r rpcResponse
	if err := json.Unmarshal(raw, &r); err != nil {
		return fmt.Errorf("nearrpc: %s: unexpected response (HTTP %d)", method, resp.StatusCode)
	}
	if r.Error != nil {
		switch r.Error.Cause.Name {
		case "UNKNOWN_ACCOUNT":
			return ErrUnknownAccount
		case "UNKNOWN_ACCESS_KEY":
			return ErrUnknownAccessKey
		}
		data := r.Error.Data
		if len(data) == 0 || string(data) == "null" {
			data = r.Error.Cause.Info
		}
		return &Error{Method: method, Name: r.Error.Name, Cause: r.Error.Cause.Name, Message: r.Error.Message, Data: data}
	}
	if resp.StatusCode != http.StatusOK || len(r.Result) == 0 {
		return fmt.Errorf("nearrpc: %s: unexpected response (HTTP %d)", method, resp.StatusCode)
	}
	if err := json.Unmarshal(r.Result, out); err != nil {
		return fmt.Errorf("nearrpc: %s: decode result: %w", method, err)
	}
	return nil
}

// ErrContractView is returned when a view call fails in the contract, e.g. a
// missing method or a panic.
var ErrContractView = errors.New("nearrpc: contract view failed")

// CallFunction calls a view method at the latest final block and returns its
// raw result and the block height it was read at.
func (c *Client) CallFunction(ctx context.Context, contractID, method string, args []byte) ([]byte, uint64, error) {
	var res struct {
		Result      []byte `json:"-"`
		RawResult   []int  `json:"result"`
		BlockHeight uint64 `json:"block_height"`
		Error       string `json:"error"`
	}
	err := c.call(ctx, "query", map[string]string{
		"request_type": "call_function",
		"finality":     "final",
		"account_id":   contractID,
		"method_name":  method,
		"args_base64":  base64.StdEncoding.EncodeToString(args),
	}, &res)
	var rpcErr *Error
	if errors.As(err, &rpcErr) && rpcErr.Cause == "CONTRACT_EXECUTION_ERROR" {
		return nil, 0, fmt.Errorf("%w: %s.%s: %s", ErrContractView, contractID, method, rpcErr.Message)
	}
	if err != nil {
		return nil, 0, err
	}
	if res.Error != "" {
		return nil, 0, fmt.Errorf("%w: %s.%s: %s", ErrContractView, contractID, method, res.Error)
	}
	out := make([]byte, len(res.RawResult))
	for i, b := range res.RawResult {
		out[i] = byte(b)
	}
	return out, res.BlockHeight, nil
}

// AccessKeyNonce returns the nonce of an access key at the latest block.
func (c *Client) AccessKeyNonce(ctx context.Context, accountID, publicKey string) (uint64, error) {
	var res struct {
		Nonce uint64 `json:"nonce"`
		Error string `json:"error"`
	}
	err := c.call(ctx, "query", map[string]string{
		"request_type": "view_access_key",
		"finality":     "optimistic",
		"account_id":   accountID,
		"public_key":   publicKey,
	}, &res)
	if err != nil {
		return 0, err
	}
	if res.Error != "" {
		if strings.HasPrefix(res.Error, "access key") {
			return 0, ErrUnknownAccessKey
		}
		return 0, fmt.Errorf("nearrpc: view_access_key: %s", res.Error)
	}
	return res.Nonce, nil
}

// Block is a block header.
type Block struct {
	Hash      string
	Height    uint64
	Timestamp uint64 // nanoseconds
}

// FinalBlock returns the latest final block.
func (c *Client) FinalBlock(ctx context.Context) (Block, error) {
	var res struct {
		Header struct {
			Hash      string `json:"hash"`
			Height    uint64 `json:"height"`
			Timestamp uint64 `json:"timestamp"`
		} `json:"header"`
	}
	if err := c.call(ctx, "block", map[string]string{"finality": "final"}, &res); err != nil {
		return Block{}, err
	}
	return Block{Hash: res.Header.Hash, Height: res.Header.Height, Timestamp: res.Header.Timestamp}, nil
}

// SendTx submits a signed transaction (Borsh) and waits until it is final.
func (c *Client) SendTx(ctx context.Context, signedTx []byte) (*Outcome, error) {
	var out Outcome
	err := c.call(ctx, "send_tx", map[string]string{
		"signed_tx_base64": base64.StdEncoding.EncodeToString(signedTx),
		"wait_until":       "FINAL",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// TxStatus returns the outcome of a transaction once it is final.
func (c *Client) TxStatus(ctx context.Context, txHash, senderID string) (*Outcome, error) {
	var out Outcome
	err := c.call(ctx, "tx", map[string]string{
		"tx_hash":           txHash,
		"sender_account_id": senderID,
		"wait_until":        "FINAL",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}
