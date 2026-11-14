package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sync"

	"github.com/harundarat/vecrow/backend/internal/domain"
	"github.com/harundarat/vecrow/backend/internal/infrastructure/nearrpc"
	"github.com/harundarat/vecrow/backend/internal/neartx"
	"github.com/harundarat/vecrow/backend/internal/usecase/escrow"
)

// Gas attached to the escrow contract's methods. A release makes up to 11
// payouts of 20 TGas each (architecture.md §5.1), so it gets the maximum.
const (
	gasCreateEscrow = 50 * neartx.TGas
	gasRelease      = 300 * neartx.TGas
	gasSigned       = 50 * neartx.TGas
	gasRetryPayout  = 100 * neartx.TGas
	gasStorage      = 30 * neartx.TGas
)

// eventStandard is the NEP-297 standard of the escrow contract's events.
const eventStandard = "vecrow"

// NEARViewCaller calls view methods.
type NEARViewCaller interface {
	CallFunction(ctx context.Context, contractID, method string, args []byte) ([]byte, uint64, error)
}

// NEARSender signs and sends transactions from one account.
// *nearrpc.Signer implements it.
type NEARSender interface {
	AccountID() string
	Send(ctx context.Context, receiverID string, actions ...neartx.Action) (*nearrpc.Outcome, error)
}

// EscrowContract calls the escrow contract. Transactions are signed by the
// relayer account, which pays the gas and gains no rights in the contract.
type EscrowContract struct {
	id      string
	rpc     NEARViewCaller
	relayer NEARSender
}

var _ escrow.Contract = (*EscrowContract)(nil)

func NewEscrowContract(contractID string, rpc NEARViewCaller, relayer NEARSender) *EscrowContract {
	return &EscrowContract{id: contractID, rpc: rpc, relayer: relayer}
}

func (c *EscrowContract) ID() string { return c.id }

func (c *EscrowContract) GetEscrow(ctx context.Context, escrowID string) (*domain.ChainEscrow, error) {
	args, err := json.Marshal(map[string]string{"escrow_id": escrowID})
	if err != nil {
		return nil, err
	}
	out, height, err := c.rpc.CallFunction(ctx, c.id, "get_escrow", args)
	if err != nil {
		return nil, fmt.Errorf("escrow contract: get_escrow: %w", err)
	}
	e, err := domain.ParseEscrowView(out, height)
	if err != nil {
		return nil, fmt.Errorf("escrow contract: %w", err)
	}
	return e, nil
}

func (c *EscrowContract) CreateEscrow(ctx context.Context, t domain.EscrowTerms, f domain.FreelancerSignature, clientSignature string) (domain.Transaction, error) {
	if f.Signature == "" || clientSignature == "" {
		// Never reached through the use cases; a guard so no escrow is
		// ever sent without both signatures.
		return domain.Transaction{}, errors.New("escrow contract: create_escrow needs both signatures")
	}
	return c.call(ctx, t.EscrowID, "create_escrow", map[string]any{
		"terms":                t.JSON(),
		"freelancer_signature": map[string]string{"public_key": f.PublicKey, "signature": f.Signature},
		"client_signature":     clientSignature,
	}, gasCreateEscrow)
}

func (c *EscrowContract) ApproveMilestone(ctx context.Context, escrowID string, milestone uint32, a escrow.ApproverAction) (domain.Transaction, error) {
	return c.call(ctx, escrowID, "approve_milestone", signedArgs(escrowID, &milestone, a), gasRelease)
}

func (c *EscrowContract) RequestRevision(ctx context.Context, escrowID string, milestone uint32, a escrow.ApproverAction) (domain.Transaction, error) {
	return c.call(ctx, escrowID, "request_revision", signedArgs(escrowID, &milestone, a), gasSigned)
}

func (c *EscrowContract) Cancel(ctx context.Context, escrowID string, a escrow.ApproverAction) (domain.Transaction, error) {
	return c.call(ctx, escrowID, "cancel", signedArgs(escrowID, nil, a), gasSigned)
}

func (c *EscrowContract) AutoRelease(ctx context.Context, escrowID string, milestone uint32) (domain.Transaction, error) {
	return c.call(ctx, escrowID, "auto_release", map[string]any{"escrow_id": escrowID, "milestone": milestone}, gasRelease)
}

func (c *EscrowContract) RetryPayout(ctx context.Context, escrowID, accountID string) (domain.Transaction, error) {
	return c.call(ctx, escrowID, "retry_payout", map[string]any{"escrow_id": escrowID, "account_id": accountID}, gasRetryPayout)
}

// Relay sends the freelancer's signed delegate action in a transaction from
// the relayer to the freelancer's account (NEP-366).
func (c *EscrowContract) Relay(ctx context.Context, escrowID string, a neartx.SignedDelegateAction) (domain.Transaction, error) {
	method := ""
	if len(a.DelegateAction.Actions) == 1 {
		if fc, ok := a.DelegateAction.Actions[0].(neartx.FunctionCall); ok {
			method = fc.MethodName
		}
	}
	out, err := c.relayer.Send(ctx, a.DelegateAction.SenderID, neartx.Delegate{SignedDelegateAction: a})
	if err != nil {
		return domain.Transaction{}, fmt.Errorf("escrow contract: relay %s: %w", method, err)
	}
	tx := transaction(out, c.id, escrowID, method, c.relayer.AccountID())
	tx.SenderID = a.DelegateAction.SenderID
	return tx, nil
}

func (c *EscrowContract) call(ctx context.Context, escrowID, method string, args any, gas uint64) (domain.Transaction, error) {
	b, err := json.Marshal(args)
	if err != nil {
		return domain.Transaction{}, fmt.Errorf("escrow contract: encode %s args: %w", method, err)
	}
	out, err := c.relayer.Send(ctx, c.id, neartx.FunctionCall{MethodName: method, Args: b, Gas: gas, Deposit: new(big.Int)})
	if err != nil {
		return domain.Transaction{}, fmt.Errorf("escrow contract: %s: %w", method, err)
	}
	return transaction(out, c.id, escrowID, method, c.relayer.AccountID()), nil
}

// signedArgs are the arguments of a method authorized by the approver's
// signature; milestone is nil for cancel.
func signedArgs(escrowID string, milestone *uint32, a escrow.ApproverAction) map[string]any {
	args := map[string]any{
		"escrow_id":   escrowID,
		"nonce":       a.Nonce,
		"deadline_ms": a.DeadlineMS,
		"signature":   a.Signature,
	}
	if milestone != nil {
		args["milestone"] = *milestone
	}
	return args
}

// transaction reports what a transaction did on contractID.
func transaction(out *nearrpc.Outcome, contractID, escrowID, method, signer string) domain.Transaction {
	res := out.CallResult(contractID, eventStandard)
	tx := domain.Transaction{
		Hash:       res.Hash,
		EscrowID:   escrowID,
		Method:     method,
		SignerID:   signer,
		SenderID:   signer,
		ReceiverID: contractID,
		Succeeded:  res.Succeeded,
		Error:      res.Failure,
	}
	for _, ev := range res.Events {
		tx.Events = append(tx.Events, domain.ContractEvent{
			ReceiptID: ev.ReceiptID, LogIndex: ev.LogIndex, Event: ev.Event, Data: ev.Data,
		})
	}
	return tx
}

// FTStorage registers accounts on a NEP-141 token (NEP-145) so they can
// receive payouts. The storage account pays the deposit (R-20).
type FTStorage struct {
	token string
	rpc   NEARViewCaller
	payer NEARSender

	mu      sync.Mutex
	deposit *big.Int // storage_balance_bounds().min, read once
}

var _ escrow.Storage = (*FTStorage)(nil)

func NewFTStorage(tokenID string, rpc NEARViewCaller, payer NEARSender) *FTStorage {
	return &FTStorage{token: tokenID, rpc: rpc, payer: payer}
}

func (s *FTStorage) IsRegistered(ctx context.Context, accountID string) (bool, error) {
	args, err := json.Marshal(map[string]string{"account_id": accountID})
	if err != nil {
		return false, err
	}
	out, _, err := s.rpc.CallFunction(ctx, s.token, "storage_balance_of", args)
	if err != nil {
		return false, fmt.Errorf("ft storage: storage_balance_of: %w", err)
	}
	var balance *struct {
		Total string `json:"total"`
	}
	if err := json.Unmarshal(out, &balance); err != nil {
		return false, fmt.Errorf("ft storage: decode storage_balance_of: %w", err)
	}
	return balance != nil, nil
}

// Register pays the minimum storage deposit for accountID with
// registration_only, so nothing is deposited twice.
func (s *FTStorage) Register(ctx context.Context, escrowID, accountID string) (domain.Transaction, error) {
	deposit, err := s.minDeposit(ctx)
	if err != nil {
		return domain.Transaction{}, err
	}
	args, err := json.Marshal(map[string]any{"account_id": accountID, "registration_only": true})
	if err != nil {
		return domain.Transaction{}, err
	}
	out, err := s.payer.Send(ctx, s.token, neartx.FunctionCall{
		MethodName: "storage_deposit", Args: args, Gas: gasStorage, Deposit: deposit,
	})
	if err != nil {
		return domain.Transaction{}, fmt.Errorf("ft storage: storage_deposit: %w", err)
	}
	return transaction(out, s.token, escrowID, "storage_deposit", s.payer.AccountID()), nil
}

func (s *FTStorage) minDeposit(ctx context.Context) (*big.Int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.deposit != nil {
		return new(big.Int).Set(s.deposit), nil
	}
	out, _, err := s.rpc.CallFunction(ctx, s.token, "storage_balance_bounds", []byte("{}"))
	if err != nil {
		return nil, fmt.Errorf("ft storage: storage_balance_bounds: %w", err)
	}
	var bounds struct {
		Min string `json:"min"`
	}
	if err := json.Unmarshal(out, &bounds); err != nil {
		return nil, fmt.Errorf("ft storage: decode storage_balance_bounds: %w", err)
	}
	min, ok := new(big.Int).SetString(bounds.Min, 10)
	if !ok || min.Sign() < 0 {
		return nil, fmt.Errorf("ft storage: storage_balance_bounds min %q", bounds.Min)
	}
	s.deposit = min
	return new(big.Int).Set(min), nil
}
