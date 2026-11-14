package nearrpc

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/mr-tron/base58"

	"github.com/harundarat/vecrow/backend/internal/infrastructure/cache"
	"github.com/harundarat/vecrow/backend/internal/neartx"
)

// ErrUnknownOutcome means a transaction was sent but its outcome could not be
// read in time: it may or may not have executed. Callers retry later; the
// contract rejects calls that are no longer valid.
var ErrUnknownOutcome = errors.New("nearrpc: transaction outcome unknown")

// ErrRejected means the node rejected the transaction before execution, e.g.
// for a lack of balance; nothing happened on chain.
var ErrRejected = errors.New("nearrpc: transaction rejected")

// NonceAllocator hands out access key nonces, so that senders sharing a key,
// in this process or others, never use the same nonce.
type NonceAllocator interface {
	// Next returns a nonce above floor that was never returned before for
	// key.
	Next(ctx context.Context, key string, floor uint64) (uint64, error)
}

// LocalNonces allocates nonces within one process.
type LocalNonces struct {
	mu   sync.Mutex
	last map[string]uint64
}

func NewLocalNonces() *LocalNonces { return &LocalNonces{last: map[string]uint64{}} }

func (n *LocalNonces) Next(_ context.Context, key string, floor uint64) (uint64, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	next := max(floor, n.last[key]) + 1
	n.last[key] = next
	return next, nil
}

// CacheNonces allocates nonces across processes: each nonce is claimed in
// the store with SetNX, so only one sender gets it.
type CacheNonces struct {
	store cache.Store
	ttl   time.Duration
	mu    sync.Mutex
	last  map[string]uint64
}

// nonceClaimTTL keeps a claim long after its transaction is final, when the
// access key's nonce on chain is above it anyway.
const nonceClaimTTL = time.Hour

func NewCacheNonces(store cache.Store) *CacheNonces {
	return &CacheNonces{store: store, ttl: nonceClaimTTL, last: map[string]uint64{}}
}

// maxNonceClaims bounds the nonces tried in one call.
const maxNonceClaims = 1000

func (n *CacheNonces) Next(ctx context.Context, key string, floor uint64) (uint64, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	candidate := max(floor, n.last[key]) + 1
	for range maxNonceClaims {
		ok, err := n.store.SetNX(ctx, "near:nonce:"+key+":"+strconv.FormatUint(candidate, 10), []byte("1"), n.ttl)
		if err != nil {
			return 0, fmt.Errorf("nearrpc: claim nonce: %w", err)
		}
		if ok {
			n.last[key] = candidate
			return candidate, nil
		}
		candidate++
	}
	return 0, errors.New("nearrpc: no free nonce")
}

// Signer sends transactions signed with one access key of an account.
type Signer struct {
	rpc       *Client
	accountID string
	key       ed25519.PrivateKey
	publicKey neartx.PublicKey
	nonces    NonceAllocator
	// awaitTimeout bounds how long a transaction whose send timed out is
	// looked up.
	awaitTimeout time.Duration
	pollInterval time.Duration

	mu          sync.Mutex
	floor       uint64
	floorLoaded bool
	block       Block
	blockAt     time.Time
}

// blockHashMaxAge bounds the reuse of a block hash for new transactions; a
// transaction stays valid for many blocks after it.
const blockHashMaxAge = 30 * time.Second

// SignerOption configures a Signer.
type SignerOption func(*Signer)

// WithAwaitTimeout bounds how long a transaction whose send timed out is
// looked up before Send returns ErrUnknownOutcome (default 2 minutes).
func WithAwaitTimeout(d time.Duration) SignerOption { return func(s *Signer) { s.awaitTimeout = d } }

// WithPollInterval sets the delay between lookups (default 1 second).
func WithPollInterval(d time.Duration) SignerOption { return func(s *Signer) { s.pollInterval = d } }

// NewSigner returns a signer for accountID with key, using rpc to send.
func NewSigner(rpc *Client, accountID string, key ed25519.PrivateKey, nonces NonceAllocator, opts ...SignerOption) *Signer {
	s := &Signer{
		rpc: rpc, accountID: accountID, key: key, publicKey: neartx.PublicKeyOf(key), nonces: nonces,
		awaitTimeout: 2 * time.Minute, pollInterval: time.Second,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// AccountID is the account that signs and pays.
func (s *Signer) AccountID() string { return s.accountID }

// PublicKey is the signing key.
func (s *Signer) PublicKey() neartx.PublicKey { return s.publicKey }

// maxSendAttempts bounds resends after a stale nonce or block hash.
const maxSendAttempts = 4

// Send signs a transaction to receiverID with actions, sends it and waits
// until it is final. A stale nonce or block hash is refreshed and the
// transaction re-signed; the node rejects those before execution.
func (s *Signer) Send(ctx context.Context, receiverID string, actions ...neartx.Action) (*Outcome, error) {
	for attempt := 1; ; attempt++ {
		signed, err := s.sign(ctx, receiverID, actions)
		if err != nil {
			return nil, err
		}
		out, err := s.rpc.SendTx(ctx, signed.Encode())
		if err == nil {
			return out, nil
		}
		var rpcErr *Error
		if errors.As(err, &rpcErr) && (rpcErr.Name == "REQUEST_VALIDATION_ERROR" || rpcErr.Cause == "PARSE_ERROR") {
			return nil, fmt.Errorf("%w: %w", ErrRejected, err)
		}
		if errors.As(err, &rpcErr) && rpcErr.Cause == "INVALID_TRANSACTION" {
			retry := s.recover(rpcErr)
			if !retry || attempt >= maxSendAttempts {
				return nil, fmt.Errorf("%w: %w", ErrRejected, err)
			}
			continue
		}
		// A timeout, or a transport error after the request may have reached
		// the node: the transaction may still execute.
		return s.await(ctx, signed.Hash())
	}
}

func (s *Signer) sign(ctx context.Context, receiverID string, actions []neartx.Action) (neartx.SignedTransaction, error) {
	floor, block, err := s.state(ctx)
	if err != nil {
		return neartx.SignedTransaction{}, err
	}
	nonce, err := s.nonces.Next(ctx, s.accountID+":"+s.publicKey.String(), floor)
	if err != nil {
		return neartx.SignedTransaction{}, err
	}
	hash, err := base58.Decode(block.Hash)
	if err != nil || len(hash) != 32 {
		return neartx.SignedTransaction{}, fmt.Errorf("nearrpc: invalid block hash %q", block.Hash)
	}
	tx := neartx.Transaction{
		SignerID:   s.accountID,
		PublicKey:  s.publicKey,
		Nonce:      nonce,
		ReceiverID: receiverID,
		Actions:    actions,
	}
	copy(tx.BlockHash[:], hash)
	return neartx.Sign(tx, s.key), nil
}

// state returns the access key's nonce on chain (read once, then kept) and
// a recent final block.
func (s *Signer) state(ctx context.Context) (uint64, Block, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.floorLoaded {
		n, err := s.rpc.AccessKeyNonce(ctx, s.accountID, s.publicKey.String())
		if err != nil {
			return 0, Block{}, fmt.Errorf("nearrpc: read nonce of %s: %w", s.accountID, err)
		}
		s.floor, s.floorLoaded = n, true
	}
	if s.block.Hash == "" || time.Since(s.blockAt) > blockHashMaxAge {
		b, err := s.rpc.FinalBlock(ctx)
		if err != nil {
			return 0, Block{}, err
		}
		s.block, s.blockAt = b, time.Now()
	}
	return s.floor, s.block, nil
}

// recover updates the cached nonce or block after an InvalidTxError and
// reports whether re-signing can help.
func (s *Signer) recover(e *Error) bool {
	var data struct {
		TxExecutionError struct {
			InvalidTxError json.RawMessage `json:"InvalidTxError"`
		} `json:"TxExecutionError"`
	}
	if json.Unmarshal(e.Data, &data) != nil {
		return false
	}
	var invalid struct {
		InvalidNonce *struct {
			AKNonce uint64 `json:"ak_nonce"`
		} `json:"InvalidNonce"`
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if json.Unmarshal(data.TxExecutionError.InvalidTxError, &invalid) == nil && invalid.InvalidNonce != nil {
		s.floor = max(s.floor, invalid.InvalidNonce.AKNonce)
		return true
	}
	var name string
	if json.Unmarshal(data.TxExecutionError.InvalidTxError, &name) == nil && name == "Expired" {
		s.block = Block{}
		return true
	}
	return false
}

// await looks the transaction up until it is final or awaitTimeout passes.
// It keeps going when ctx is cancelled, so the outcome is still known.
func (s *Signer) await(ctx context.Context, hash string) (*Outcome, error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.awaitTimeout)
	defer cancel()
	for {
		out, err := s.rpc.TxStatus(ctx, hash, s.accountID)
		if err == nil {
			return out, nil
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("%w: %s: %w", ErrUnknownOutcome, hash, err)
		case <-time.After(s.pollInterval):
		}
	}
}
