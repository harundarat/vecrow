package nearrpc_test

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mr-tron/base58"

	"github.com/harundarat/vecrow/backend/internal/infrastructure/nearrpc"
	"github.com/harundarat/vecrow/backend/internal/neartx"
)

// A call to escrow.test.near through a delegate action: the transaction
// creates a receipt on the sender (alice), which creates the receipt on the
// contract. The contract's receipt logs an event and returns a promise whose
// callback returns true.
const delegateOutcome = `{
  "final_execution_status": "FINAL",
  "status": {"SuccessValue": ""},
  "transaction": {"hash": "TxHash1", "signer_id": "relayer.test.near", "receiver_id": "alice.test.near"},
  "transaction_outcome": {"id": "TxHash1", "outcome": {"logs": [], "receipt_ids": ["R1"], "status": {"SuccessReceiptId": "R1"}, "executor_id": "relayer.test.near"}},
  "receipts_outcome": [
    {"id": "R1", "outcome": {"logs": [], "receipt_ids": ["R2", "Refund"], "status": {"SuccessValue": ""}, "executor_id": "alice.test.near"}},
    {"id": "R2", "outcome": {"logs": ["EVENT_JSON:{\"standard\":\"vecrow\",\"version\":\"1.0.0\",\"event\":\"milestone_submitted\",\"data\":{\"escrow_id\":\"e1\",\"milestone\":0,\"submitted_at_ms\":5}}", "plain log", "EVENT_JSON:{\"standard\":\"nep141\",\"version\":\"1.0.0\",\"event\":\"ft_transfer\",\"data\":[]}"], "receipt_ids": ["R3"], "status": {"SuccessReceiptId": "R3"}, "executor_id": "escrow.test.near"}},
    {"id": "R3", "outcome": {"logs": ["EVENT_JSON:{\"standard\":\"vecrow\",\"version\":\"1.0.0\",\"event\":\"payout\",\"data\":[{\"escrow_id\":\"e1\",\"paid\":true}]}"], "receipt_ids": [], "status": {"SuccessValue": "dHJ1ZQ=="}, "executor_id": "escrow.test.near"}},
    {"id": "Refund", "outcome": {"logs": [], "receipt_ids": [], "status": {"SuccessValue": ""}, "executor_id": "relayer.test.near"}}
  ]
}`

// A direct call whose receipt panics; the top-level status is a failure.
const panicOutcome = `{
  "status": {"Failure": {"ActionError": {"index": 0, "kind": {"FunctionCallError": {"ExecutionError": "Smart contract panicked: milestone is not Submitted"}}}}},
  "transaction": {"hash": "TxHash2", "signer_id": "relayer.test.near", "receiver_id": "escrow.test.near"},
  "transaction_outcome": {"id": "TxHash2", "outcome": {"logs": [], "receipt_ids": ["P1"], "status": {"SuccessReceiptId": "P1"}, "executor_id": "relayer.test.near"}},
  "receipts_outcome": [
    {"id": "P1", "outcome": {"logs": ["EVENT_JSON:{\"standard\":\"vecrow\",\"version\":\"1.0.0\",\"event\":\"x\",\"data\":{}}"], "receipt_ids": [], "status": {"Failure": {"ActionError": {"index": 0, "kind": {"FunctionCallError": {"ExecutionError": "Smart contract panicked: milestone is not Submitted"}}}}}, "executor_id": "escrow.test.near"}}
  ]
}`

// A delegate action whose own receipt fails (the nonce was used).
const delegateFailedOutcome = `{
  "status": {"Failure": {"ActionError": {"index": 0, "kind": {"DelegateActionInvalidNonce": {"delegate_nonce": 5, "ak_nonce": 5}}}}},
  "transaction": {"hash": "TxHash3", "signer_id": "relayer.test.near", "receiver_id": "alice.test.near"},
  "transaction_outcome": {"id": "TxHash3", "outcome": {"logs": [], "receipt_ids": ["D1"], "status": {"SuccessReceiptId": "D1"}, "executor_id": "relayer.test.near"}},
  "receipts_outcome": [
    {"id": "D1", "outcome": {"logs": [], "receipt_ids": [], "status": {"Failure": {"ActionError": {"index": 0, "kind": {"DelegateActionInvalidNonce": {"delegate_nonce": 5, "ak_nonce": 5}}}}}, "executor_id": "alice.test.near"}}
  ]
}`

func decodeOutcome(t *testing.T, s string) *nearrpc.Outcome {
	t.Helper()
	var o nearrpc.Outcome
	if err := json.Unmarshal([]byte(s), &o); err != nil {
		t.Fatal(err)
	}
	return &o
}

func TestCallResultThroughDelegate(t *testing.T) {
	res := decodeOutcome(t, delegateOutcome).CallResult("escrow.test.near", "vecrow")
	if !res.Succeeded || res.Failure != "" || string(res.Value) != "true" || res.Hash != "TxHash1" {
		t.Fatalf("result = %+v", res)
	}
	if len(res.Events) != 2 {
		t.Fatalf("events = %+v", res.Events)
	}
	if e := res.Events[0]; e.Event != "milestone_submitted" || e.ReceiptID != "R2" || e.LogIndex != 0 ||
		!strings.Contains(string(e.Data), `"submitted_at_ms":5`) {
		t.Errorf("event 0 = %+v", e)
	}
	if e := res.Events[1]; e.Event != "payout" || e.ReceiptID != "R3" || !strings.HasPrefix(string(e.Data), "[") {
		t.Errorf("event 1 = %+v", e)
	}
}

func TestCallResultFailures(t *testing.T) {
	res := decodeOutcome(t, panicOutcome).CallResult("escrow.test.near", "vecrow")
	if res.Succeeded || res.Failure != "milestone is not Submitted" || len(res.Events) != 0 {
		t.Fatalf("panic result = %+v", res)
	}
	res = decodeOutcome(t, delegateFailedOutcome).CallResult("escrow.test.near", "vecrow")
	if res.Succeeded || !strings.Contains(res.Failure, "DelegateActionInvalidNonce") {
		t.Fatalf("delegate failure = %+v", res)
	}
	// A call to another contract never reached the expected one.
	res = decodeOutcome(t, delegateOutcome).CallResult("other.test.near", "vecrow")
	if res.Succeeded || !strings.Contains(res.Failure, "no receipt reached") {
		t.Fatalf("other contract = %+v", res)
	}
}

// fakeChain is a minimal RPC node for the signer: it checks signatures and
// nonces like a node does before execution.
type fakeChain struct {
	t       *testing.T
	pub     ed25519.PublicKey
	mu      sync.Mutex
	akNonce uint64
	sent    []neartx.Transaction
	// timeouts makes the next n send_tx calls time out after accepting.
	timeouts int
	// unknownLookups makes the next n tx lookups report an unknown tx.
	unknownLookups int
}

func (f *fakeChain) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var req struct {
		Method string            `json:"method"`
		Params map[string]string `json:"params"`
	}
	b, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(b, &req); err != nil {
		f.t.Errorf("bad request %s", b)
	}
	result := func(v string) { fmt.Fprintf(w, `{"jsonrpc":"2.0","id":"vecrow","result":%s}`, v) }
	rpcError := func(name, cause, data string) {
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":"vecrow","error":{"name":%q,"cause":{"name":%q},"message":"err","data":%s}}`, name, cause, data)
	}
	switch req.Method {
	case "query":
		result(fmt.Sprintf(`{"nonce":%d,"permission":"FullAccess","block_height":1,"block_hash":"h"}`, f.akNonce))
	case "block":
		result(fmt.Sprintf(`{"header":{"hash":%q,"height":10,"timestamp":1}}`, base58.Encode(make([]byte, 32))))
	case "send_tx":
		raw, _ := base64.StdEncoding.DecodeString(req.Params["signed_tx_base64"])
		tx, ok := f.parse(raw)
		if !ok {
			rpcError("HANDLER_ERROR", "INVALID_TRANSACTION", `{"TxExecutionError":{"InvalidTxError":"InvalidSignature"}}`)
			return
		}
		if tx.Nonce <= f.akNonce {
			rpcError("HANDLER_ERROR", "INVALID_TRANSACTION",
				fmt.Sprintf(`{"TxExecutionError":{"InvalidTxError":{"InvalidNonce":{"tx_nonce":%d,"ak_nonce":%d}}}}`, tx.Nonce, f.akNonce))
			return
		}
		f.akNonce = tx.Nonce
		f.sent = append(f.sent, tx)
		if f.timeouts > 0 {
			f.timeouts--
			rpcError("HANDLER_ERROR", "TIMEOUT_ERROR", `null`)
			return
		}
		result(strings.Replace(delegateOutcome, "TxHash1", "sent", 1))
	case "tx":
		if f.unknownLookups > 0 {
			f.unknownLookups--
			rpcError("HANDLER_ERROR", "UNKNOWN_TRANSACTION", `null`)
			return
		}
		result(strings.Replace(delegateOutcome, "TxHash1", req.Params["tx_hash"], 1))
	default:
		f.t.Errorf("unexpected method %s", req.Method)
	}
}

// parse checks the signature of a signed transaction encoded by neartx and
// returns its nonce. The transaction fields are re-encoded by the test from
// what the signer is expected to send.
func (f *fakeChain) parse(raw []byte) (neartx.Transaction, bool) {
	if len(raw) < 65 {
		return neartx.Transaction{}, false
	}
	body, sig := raw[:len(raw)-65], raw[len(raw)-64:]
	h := sha256Sum(body)
	if !ed25519.Verify(f.pub, h[:], sig) {
		return neartx.Transaction{}, false
	}
	// signer_id (u32 len + bytes), public key (33), nonce (u64).
	n := int(uint32(body[0]) | uint32(body[1])<<8 | uint32(body[2])<<16 | uint32(body[3])<<24)
	off := 4 + n + 33
	var nonce uint64
	for i := 7; i >= 0; i-- {
		nonce = nonce<<8 | uint64(body[off+i])
	}
	return neartx.Transaction{Nonce: nonce}, true
}

func newSigner(t *testing.T, chain *fakeChain, nonces nearrpc.NonceAllocator) *nearrpc.Signer {
	t.Helper()
	srv := httptest.NewServer(chain)
	t.Cleanup(srv.Close)
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	chain.pub = key.Public().(ed25519.PublicKey)
	return nearrpc.NewSigner(nearrpc.New(srv.URL, 5*time.Second), "relayer.test.near", key, nonces,
		nearrpc.WithPollInterval(time.Millisecond), nearrpc.WithAwaitTimeout(time.Second))
}

func call() neartx.Action {
	return neartx.FunctionCall{MethodName: "m", Args: []byte("{}"), Gas: neartx.TGas}
}

func TestSignerSendsWithIncreasingNonces(t *testing.T) {
	chain := &fakeChain{t: t, akNonce: 41}
	s := newSigner(t, chain, nearrpc.NewLocalNonces())
	for range 3 {
		out, err := s.Send(context.Background(), "escrow.test.near", call())
		if err != nil {
			t.Fatalf("Send: %v", err)
		}
		if res := out.CallResult("escrow.test.near", "vecrow"); !res.Succeeded {
			t.Fatalf("result = %+v", res)
		}
	}
	if len(chain.sent) != 3 || chain.sent[0].Nonce != 42 || chain.sent[2].Nonce != 44 {
		t.Fatalf("sent nonces = %+v", chain.sent)
	}
}

func TestSignerRecoversFromStaleNonce(t *testing.T) {
	chain := &fakeChain{t: t, akNonce: 5}
	s := newSigner(t, chain, nearrpc.NewLocalNonces())
	if _, err := s.Send(context.Background(), "escrow.test.near", call()); err != nil {
		t.Fatal(err)
	}
	// Another sender used the key: the chain is now far ahead.
	chain.mu.Lock()
	chain.akNonce = 100
	chain.mu.Unlock()
	if _, err := s.Send(context.Background(), "escrow.test.near", call()); err != nil {
		t.Fatalf("Send after external use: %v", err)
	}
	if last := chain.sent[len(chain.sent)-1].Nonce; last != 101 {
		t.Fatalf("nonce after recovery = %d, want 101", last)
	}
}

func TestSignerAwaitsTimedOutTransaction(t *testing.T) {
	chain := &fakeChain{t: t, timeouts: 1, unknownLookups: 2}
	s := newSigner(t, chain, nearrpc.NewLocalNonces())
	out, err := s.Send(context.Background(), "escrow.test.near", call())
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(chain.sent) != 1 || out.Transaction.Hash == "" || out.Transaction.Hash == "sent" {
		t.Fatalf("expected the outcome from a lookup, got hash %q after %d sends", out.Transaction.Hash, len(chain.sent))
	}

	// A transaction that never shows up has an unknown outcome.
	chain.mu.Lock()
	chain.timeouts, chain.unknownLookups = 1, 1_000_000
	chain.mu.Unlock()
	if _, err := s.Send(context.Background(), "escrow.test.near", call()); !errors.Is(err, nearrpc.ErrUnknownOutcome) {
		t.Fatalf("error = %v, want ErrUnknownOutcome", err)
	}
}

func TestSignerReportsRejection(t *testing.T) {
	chain := &fakeChain{t: t}
	s := newSigner(t, chain, nearrpc.NewLocalNonces())
	chain.pub = ed25519.NewKeyFromSeed(make32(9)).Public().(ed25519.PublicKey) // signatures no longer verify
	if _, err := s.Send(context.Background(), "escrow.test.near", call()); !errors.Is(err, nearrpc.ErrRejected) {
		t.Fatalf("error = %v, want ErrRejected", err)
	}
}

func make32(b byte) []byte {
	s := make([]byte, 32)
	for i := range s {
		s[i] = b
	}
	return s
}

type memStore struct {
	mu   sync.Mutex
	keys map[string]bool
}

func (m *memStore) Get(context.Context, string) ([]byte, error)                { return nil, nil }
func (m *memStore) Set(context.Context, string, []byte, time.Duration) error   { return nil }
func (m *memStore) GetDel(context.Context, string) ([]byte, error)             { return nil, nil }
func (m *memStore) Delete(context.Context, ...string) error                    { return nil }
func (m *memStore) Incr(context.Context, string, time.Duration) (int64, error) { return 0, nil }
func (m *memStore) SetNX(_ context.Context, key string, _ []byte, _ time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.keys[key] {
		return false, nil
	}
	m.keys[key] = true
	return true, nil
}

func TestCacheNoncesAreUniqueAcrossAllocators(t *testing.T) {
	store := &memStore{keys: map[string]bool{}}
	a, b := nearrpc.NewCacheNonces(store), nearrpc.NewCacheNonces(store)
	seen := map[uint64]bool{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, alloc := range []*nearrpc.CacheNonces{a, b} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 50 {
				n, err := alloc.Next(context.Background(), "k", 10)
				if err != nil {
					t.Error(err)
					return
				}
				mu.Lock()
				if seen[n] || n <= 10 {
					t.Errorf("nonce %d reused or below floor", n)
				}
				seen[n] = true
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if len(seen) != 100 {
		t.Fatalf("allocated %d nonces, want 100", len(seen))
	}
}

func sha256Sum(b []byte) [32]byte { return sha256.Sum256(b) }
