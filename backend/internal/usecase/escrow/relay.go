package escrow

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strconv"

	"github.com/harundarat/vecrow/backend/internal/domain"
	"github.com/harundarat/vecrow/backend/internal/neartx"
)

// Methods the freelancer calls through the relayer with a NEP-366 delegate
// action. The contract checks that the predecessor is the freelancer; the
// relayer pays the gas only for these, on the Vecrow contract (R-09).
const (
	methodSubmit = "submit_milestone"
	methodRefund = "refund_unsubmitted"
	methodCancel = "cancel"
)

// relayArgs lists the arguments each relayed method may carry.
var relayArgs = map[string][]string{
	methodSubmit: {"escrow_id", "milestone"},
	methodRefund: {"escrow_id", "milestone"},
	methodCancel: {"escrow_id"},
}

// maxDelegateBytes bounds a signed delegate action; one function call with
// small JSON arguments is a few hundred bytes.
const maxDelegateBytes = 4096

// ActionResult is the transaction an action sent and the escrow after it.
type ActionResult struct {
	Escrow      domain.Escrow
	Transaction domain.Transaction
}

// DecodeDelegate decodes a base64 Borsh SignedDelegateAction, as wallets
// return it.
func DecodeDelegate(s string) ([]byte, error) {
	if s == "" {
		return nil, domain.InvalidArgument("delegate_action is required")
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		b, err = base64.RawURLEncoding.DecodeString(s)
	}
	if err != nil || len(b) > maxDelegateBytes {
		return nil, domain.InvalidArgument("delegate_action must be base64 of a Borsh SignedDelegateAction")
	}
	return b, nil
}

// checkDelegate applies the relayer policy: the delegate action is signed by
// the escrow's freelancer, goes to the Vecrow contract, and holds exactly one
// function call to method, with no deposit, gas within the cap, and
// arguments for this escrow that want accepts. Anything else is refused
// before the relayer spends gas.
func (s *Service) checkDelegate(raw []byte, t domain.EscrowTerms, method string,
	want func(args map[string]json.RawMessage) error) (neartx.SignedDelegateAction, error) {
	sda, err := neartx.DecodeSignedDelegateAction(raw)
	if err != nil {
		return sda, domain.InvalidArgument("delegate_action is malformed: %v", err)
	}
	da := sda.DelegateAction
	if da.ReceiverID != s.ContractID() {
		return sda, domain.Forbidden("the relayer only forwards calls to the Vecrow contract %s", s.ContractID())
	}
	if len(da.Actions) != 1 {
		return sda, domain.Forbidden("the delegate action must hold exactly one function call")
	}
	fc, ok := da.Actions[0].(neartx.FunctionCall)
	if !ok {
		return sda, domain.Forbidden("the delegate action must hold a function call")
	}
	if _, allowed := relayArgs[fc.MethodName]; !allowed || fc.MethodName != method {
		return sda, domain.Forbidden("this endpoint relays only %s, not %q", method, fc.MethodName)
	}
	if fc.Deposit != nil && fc.Deposit.Sign() != 0 {
		return sda, domain.Forbidden("the function call must attach no deposit")
	}
	if fc.Gas == 0 || fc.Gas > s.cfg.RelayerMaxGas {
		return sda, domain.Forbidden("the function call gas must be between 1 and %d", s.cfg.RelayerMaxGas)
	}
	if da.SenderID != t.Freelancer {
		return sda, domain.Forbidden("only the freelancer %s can act through the relayer on this escrow", t.Freelancer)
	}

	var args map[string]json.RawMessage
	dec := json.NewDecoder(bytes.NewReader(fc.Args))
	if err := dec.Decode(&args); err != nil || dec.More() {
		return sda, domain.InvalidArgument("the function call arguments must be a JSON object")
	}
	for k := range args {
		if !slices.Contains(relayArgs[method], k) {
			return sda, domain.InvalidArgument("unexpected argument %q for %s", k, method)
		}
	}
	var escrowID string
	if err := json.Unmarshal(args["escrow_id"], &escrowID); err != nil || escrowID != t.EscrowID {
		return sda, domain.InvalidArgument("the function call is not for escrow %s", t.EscrowID)
	}
	if err := want(args); err != nil {
		return sda, err
	}
	if !sda.Verify() {
		return sda, domain.Unauthorized("the delegate action's signature is invalid")
	}
	return sda, nil
}

// milestoneArg returns the "milestone" argument, or nil when it is absent or
// null.
func milestoneArg(args map[string]json.RawMessage) (*uint32, error) {
	raw, ok := args["milestone"]
	if !ok || string(raw) == "null" {
		return nil, nil
	}
	n, err := strconv.ParseUint(string(raw), 10, 32)
	if err != nil {
		return nil, domain.InvalidArgument("milestone must be a non-negative integer")
	}
	m := uint32(n)
	return &m, nil
}

// onChain returns the escrow and its signed terms if it is on-chain.
func (s *Service) onChain(ctx context.Context, id string) (domain.Escrow, domain.EscrowTerms, error) {
	e, err := s.get(ctx, id)
	if err != nil {
		return domain.Escrow{}, domain.EscrowTerms{}, err
	}
	t, ok := e.Terms()
	if !e.OnChain() || !ok {
		return domain.Escrow{}, domain.EscrowTerms{}, domain.Conflict("escrow is %s, not on-chain yet", e.Status)
	}
	return e, t, nil
}

func checkMilestone(e domain.Escrow, milestone uint32) error {
	if int(milestone) >= len(e.Milestones) {
		return domain.NotFound("milestone %d does not exist", milestone)
	}
	return nil
}

// SubmitMilestone relays the freelancer's submit_milestone (R-05, R-09).
func (s *Service) SubmitMilestone(ctx context.Context, id string, milestone uint32, delegate []byte) (ActionResult, error) {
	e, t, err := s.onChain(ctx, id)
	if err != nil {
		return ActionResult{}, err
	}
	if err := checkMilestone(e, milestone); err != nil {
		return ActionResult{}, err
	}
	sda, err := s.checkDelegate(delegate, t, methodSubmit, func(args map[string]json.RawMessage) error {
		m, err := milestoneArg(args)
		if err != nil {
			return err
		}
		if m == nil || *m != milestone {
			return domain.InvalidArgument("the function call is not for milestone %d", milestone)
		}
		return nil
	})
	if err != nil {
		return ActionResult{}, err
	}
	return s.finish(ctx, id, "submit", func() (domain.Transaction, error) { return s.Contract.Relay(ctx, id, sda) })
}

// RefundUnsubmitted relays the freelancer's refund_unsubmitted, for one
// milestone or every Pending one (R-18, R-09).
func (s *Service) RefundUnsubmitted(ctx context.Context, id string, delegate []byte) (ActionResult, error) {
	e, t, err := s.onChain(ctx, id)
	if err != nil {
		return ActionResult{}, err
	}
	sda, err := s.checkDelegate(delegate, t, methodRefund, func(args map[string]json.RawMessage) error {
		m, err := milestoneArg(args)
		if err != nil {
			return err
		}
		if m != nil {
			return checkMilestone(e, *m)
		}
		return nil
	})
	if err != nil {
		return ActionResult{}, err
	}
	return s.finish(ctx, id, "refund", func() (domain.Transaction, error) { return s.Contract.Relay(ctx, id, sda) })
}

// checkApproverAction checks the nonce, deadline and the approver's
// signature over the action message.
func (s *Service) checkApproverAction(t domain.EscrowTerms, a ApproverAction, message func(nonce [domain.NonceSize]byte) string) error {
	nonce, err := domain.DecodeNonce(a.Nonce)
	if err != nil {
		return err
	}
	if a.DeadlineMS <= s.nowMS() {
		return domain.InvalidArgument("deadline_ms has passed")
	}
	if a.Signature == "" {
		return domain.InvalidArgument("signature is required")
	}
	return s.verifyApproverAction(t.Approver, message(nonce), nonce, a.Signature)
}

// ApproveMilestone forwards the approver's signed approval (R-06, R-09).
func (s *Service) ApproveMilestone(ctx context.Context, id string, milestone uint32, a ApproverAction) (ActionResult, error) {
	e, t, err := s.onChain(ctx, id)
	if err != nil {
		return ActionResult{}, err
	}
	if err := checkMilestone(e, milestone); err != nil {
		return ActionResult{}, err
	}
	err = s.checkApproverAction(t, a, func(n [domain.NonceSize]byte) string {
		return domain.MilestoneActionMessage(domain.ActionApproveMilestone, s.ContractID(), id, milestone, n, a.DeadlineMS)
	})
	if err != nil {
		return ActionResult{}, err
	}
	return s.finish(ctx, id, "approve", func() (domain.Transaction, error) {
		return s.Contract.ApproveMilestone(ctx, id, milestone, a)
	})
}

// RequestRevision forwards the approver's signed revision request and keeps
// its reason (R-17, R-09).
func (s *Service) RequestRevision(ctx context.Context, id string, milestone uint32, a ApproverAction, reason string) (ActionResult, error) {
	reason, err := domain.ParseRevisionReason(reason)
	if err != nil {
		return ActionResult{}, err
	}
	e, t, err := s.onChain(ctx, id)
	if err != nil {
		return ActionResult{}, err
	}
	if err := checkMilestone(e, milestone); err != nil {
		return ActionResult{}, err
	}
	err = s.checkApproverAction(t, a, func(n [domain.NonceSize]byte) string {
		return domain.MilestoneActionMessage(domain.ActionRequestRevision, s.ContractID(), id, milestone, n, a.DeadlineMS)
	})
	if err != nil {
		return ActionResult{}, err
	}
	res, err := s.finish(ctx, id, "revision", func() (domain.Transaction, error) {
		return s.Contract.RequestRevision(ctx, id, milestone, a)
	})
	if err != nil {
		return ActionResult{}, err
	}
	note := domain.RevisionNote{Milestone: int(milestone), Reason: reason, TxHash: res.Transaction.Hash}
	if err := s.Repo.AddRevisionNote(ctx, id, note); err != nil {
		return ActionResult{}, fmt.Errorf("escrow: add revision note: %w", err)
	}
	return res, nil
}

// CancelInput is a cancellation: by the creator's session before the escrow
// is on-chain, then by the freelancer's delegate action or the approver's
// signature.
type CancelInput struct {
	AccountID string
	// Delegate is the freelancer's signed delegate action for cancel.
	Delegate []byte
	// Approver is the approver's signed cancellation.
	Approver *ApproverAction
}

// Cancel cancels an escrow before it is funded (R-01, R-23).
func (s *Service) Cancel(ctx context.Context, id string, in CancelInput) (ActionResult, error) {
	e, err := s.get(ctx, id)
	if err != nil {
		return ActionResult{}, err
	}
	switch {
	case e.Status == domain.EscrowApproved:
		return ActionResult{}, domain.Conflict("escrow is being created on-chain; cancel it once it is Created")
	case !e.OnChain():
		if in.Delegate != nil || in.Approver != nil {
			return ActionResult{}, domain.InvalidArgument("escrow is not on-chain; its creator cancels it with a session")
		}
		if in.AccountID == "" {
			return ActionResult{}, domain.Unauthorized("a session of the creator is required")
		}
		e, err := s.cancelOffChain(ctx, in.AccountID, id)
		return ActionResult{Escrow: e}, err
	case e.Status != domain.EscrowCreated:
		return ActionResult{}, domain.Conflict("escrow is %s; only a Created escrow can be cancelled", e.Status)
	}

	t, _ := e.Terms()
	switch {
	case in.Delegate != nil && in.Approver == nil:
		sda, err := s.checkDelegate(in.Delegate, t, methodCancel, func(map[string]json.RawMessage) error { return nil })
		if err != nil {
			return ActionResult{}, err
		}
		return s.finish(ctx, id, "cancel", func() (domain.Transaction, error) { return s.Contract.Relay(ctx, id, sda) })
	case in.Approver != nil && in.Delegate == nil:
		a := *in.Approver
		err := s.checkApproverAction(t, a, func(n [domain.NonceSize]byte) string {
			return domain.CancelMessage(s.ContractID(), id, n, a.DeadlineMS)
		})
		if err != nil {
			return ActionResult{}, err
		}
		return s.finish(ctx, id, "cancel", func() (domain.Transaction, error) { return s.Contract.Cancel(ctx, id, a) })
	default:
		return ActionResult{}, domain.InvalidArgument("give either the freelancer's delegate_action or the approver's nonce, deadline_ms and signature")
	}
}

// finish sends a transaction, records it with its events, refreshes the
// escrow from the contract, and reports a rejected call as a conflict.
func (s *Service) finish(ctx context.Context, id, action string, send func() (domain.Transaction, error)) (ActionResult, error) {
	tx, err := send()
	if err != nil {
		return ActionResult{}, s.sendError(err)
	}
	if err := s.record(ctx, id, tx); err != nil {
		return ActionResult{}, err
	}
	if err := s.Sync(ctx, id); err != nil {
		s.rt.log.WarnContext(ctx, "escrow refresh after transaction failed",
			slog.String("escrow_id", id), slog.String("error", err.Error()))
	}
	if !tx.Succeeded {
		return ActionResult{}, domain.Conflict("the contract rejected %s: %s", action, tx.Error)
	}
	s.rt.log.InfoContext(ctx, "escrow action relayed",
		slog.String("escrow_id", id), slog.String("action", action), slog.String("tx_hash", tx.Hash))
	e, err := s.get(ctx, id)
	if err != nil {
		return ActionResult{}, err
	}
	return ActionResult{Escrow: e, Transaction: tx}, nil
}

// sendError reports a transaction that was not sent, or whose outcome is
// unknown, as temporarily unavailable.
func (s *Service) sendError(err error) error {
	return fmt.Errorf("%w (%w)", domain.Unavailable("the NEAR transaction could not be confirmed; check the escrow before retrying"), err)
}

// record stores a transaction the backend sent for escrow id.
func (s *Service) record(ctx context.Context, id string, tx domain.Transaction) error {
	tx.EscrowID = id
	if err := s.Repo.SaveTransaction(ctx, tx); err != nil {
		return fmt.Errorf("escrow: save transaction %s: %w", tx.Hash, err)
	}
	return nil
}

// eventNames lists the event names of a transaction, for logs.
func eventNames(tx domain.Transaction) []string {
	names := map[string]bool{}
	for _, e := range tx.Events {
		names[e.Event] = true
	}
	return slices.Sorted(maps.Keys(names))
}
