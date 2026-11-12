// Package oneclick is a client for the NEAR Intents 1Click Swap API
// (https://docs.near-intents.org/integration/distribution-channels/1click-api):
// the token list, quotes and the status of a swap by its deposit address.
package oneclick

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultBaseURL is the public 1Click API.
const DefaultBaseURL = "https://1click.chaindefuser.com"

// maxResponseBytes bounds responses read from 1Click; the token list is the
// largest, at a few hundred kilobytes.
const maxResponseBytes = 8 << 20

// ErrNotFound is returned by Status for an unknown deposit address.
var ErrNotFound = errors.New("oneclick: not found")

// Error is an error response of the API.
type Error struct {
	StatusCode int
	// Message is the API's message, e.g. why a quote was refused.
	Message string
}

func (e *Error) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("oneclick: HTTP %d", e.StatusCode)
	}
	return fmt.Sprintf("oneclick: HTTP %d: %s", e.StatusCode, e.Message)
}

// Client calls the 1Click API. The API key, if set, is sent as a bearer
// token; quotes without one pay an extra fee.
type Client struct {
	base   string
	apiKey string
	http   *http.Client
}

// New returns a client for baseURL. httpClient may be nil.
func New(baseURL, apiKey string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{base: strings.TrimRight(baseURL, "/"), apiKey: apiKey, http: httpClient}
}

// Number is a JSON number that 1Click may also send as a string.
type Number float64

func (n *Number) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*n = 0
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fmt.Errorf("oneclick: number %s: %w", b, err)
	}
	*n = Number(f)
	return nil
}

// Token is a token 1Click can swap.
type Token struct {
	AssetID         string `json:"assetId"`
	Decimals        int    `json:"decimals"`
	Blockchain      string `json:"blockchain"`
	Symbol          string `json:"symbol"`
	Price           Number `json:"price"`
	ContractAddress string `json:"contractAddress,omitempty"`
}

// Tokens returns the supported tokens (GET /v0/tokens).
func (c *Client) Tokens(ctx context.Context) ([]Token, error) {
	var out []Token
	if _, err := c.do(ctx, http.MethodGet, "/v0/tokens", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Swap types, deposit types and recipient types of a quote request.
const (
	SwapExactOutput           = "EXACT_OUTPUT"
	DepositOriginChain        = "ORIGIN_CHAIN"
	RefundOriginChain         = "ORIGIN_CHAIN"
	RecipientDestinationChain = "DESTINATION_CHAIN"
)

// QuoteRequest is the body of POST /v0/quote.
type QuoteRequest struct {
	Dry                bool      `json:"dry"`
	SwapType           string    `json:"swapType"`
	SlippageTolerance  int       `json:"slippageTolerance"`
	OriginAsset        string    `json:"originAsset"`
	DepositType        string    `json:"depositType"`
	DestinationAsset   string    `json:"destinationAsset"`
	Amount             string    `json:"amount"`
	RefundTo           string    `json:"refundTo"`
	RefundType         string    `json:"refundType"`
	Recipient          string    `json:"recipient"`
	RecipientType      string    `json:"recipientType"`
	Deadline           time.Time `json:"deadline"`
	CustomRecipientMsg string    `json:"customRecipientMsg,omitempty"`
}

// Quote is the quote of a QuoteResponse.
type Quote struct {
	DepositAddress     string     `json:"depositAddress"`
	DepositMemo        string     `json:"depositMemo,omitempty"`
	AmountIn           string     `json:"amountIn"`
	AmountInFormatted  string     `json:"amountInFormatted"`
	MinAmountIn        string     `json:"minAmountIn,omitempty"`
	AmountOut          string     `json:"amountOut"`
	AmountOutFormatted string     `json:"amountOutFormatted"`
	MinAmountOut       string     `json:"minAmountOut,omitempty"`
	TimeEstimate       Number     `json:"timeEstimate"`
	Deadline           *time.Time `json:"deadline,omitempty"`
}

// QuoteResponse is the response of POST /v0/quote.
type QuoteResponse struct {
	CorrelationID string `json:"correlationId"`
	Quote         Quote  `json:"quote"`
	// Raw is the response body as received.
	Raw []byte `json:"-"`
}

// Quote requests a quote; with Dry false it reserves a deposit address.
func (c *Client) Quote(ctx context.Context, req QuoteRequest) (QuoteResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return QuoteResponse{}, fmt.Errorf("oneclick: encode quote request: %w", err)
	}
	var out QuoteResponse
	raw, err := c.do(ctx, http.MethodPost, "/v0/quote", body, &out)
	if err != nil {
		return QuoteResponse{}, err
	}
	out.Raw = raw
	return out, nil
}

// Swap statuses.
const (
	StatusKnownDepositTx    = "KNOWN_DEPOSIT_TX"
	StatusPendingDeposit    = "PENDING_DEPOSIT"
	StatusIncompleteDeposit = "INCOMPLETE_DEPOSIT"
	StatusProcessing        = "PROCESSING"
	StatusSuccess           = "SUCCESS"
	StatusRefunded          = "REFUNDED"
	StatusFailed            = "FAILED"
)

// StatusResponse is the response of GET /v0/status.
type StatusResponse struct {
	Status      string    `json:"status"`
	UpdatedAt   time.Time `json:"updatedAt"`
	SwapDetails struct {
		NearTxHashes   []string `json:"nearTxHashes"`
		AmountOut      string   `json:"amountOut,omitempty"`
		RefundedAmount string   `json:"refundedAmount,omitempty"`
		RefundReason   string   `json:"refundReason,omitempty"`
	} `json:"swapDetails"`
}

// Status returns the status of the swap with the deposit address (and memo,
// for chains that use one). An unknown address is ErrNotFound.
func (c *Client) Status(ctx context.Context, depositAddress, depositMemo string) (StatusResponse, error) {
	q := url.Values{"depositAddress": {depositAddress}}
	if depositMemo != "" {
		q.Set("depositMemo", depositMemo)
	}
	var out StatusResponse
	_, err := c.do(ctx, http.MethodGet, "/v0/status?"+q.Encode(), nil, &out)
	var apiErr *Error
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
		return StatusResponse{}, fmt.Errorf("%w: %w", ErrNotFound, err)
	}
	return out, err
}

// do sends a request and decodes a 2xx JSON response into out, returning
// the body.
func (c *Client) do(ctx context.Context, method, path string, body []byte, out any) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, reader)
	if err != nil {
		return nil, fmt.Errorf("oneclick: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("oneclick: %s %s: %w", method, strings.SplitN(path, "?", 2)[0], err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("oneclick: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		apiErr := &Error{StatusCode: resp.StatusCode}
		var msg struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(b, &msg) == nil {
			apiErr.Message = msg.Message
		}
		return nil, apiErr
	}
	if err := json.Unmarshal(b, out); err != nil {
		return nil, fmt.Errorf("oneclick: decode response: %w", err)
	}
	return b, nil
}
