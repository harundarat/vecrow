package escrow

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/harundarat/vecrow/backend/internal/domain"
)

// deadlineSkew tolerates clock differences when a submitted deadline is
// compared with the latest one the backend would have built.
const deadlineSkew = time.Minute

// Terms are final terms with the message to sign.
type Terms struct {
	Terms   domain.EscrowTerms
	Message string
	// Nonce and Recipient are what a NEAR wallet passes to signMessage for
	// the terms: sha256(message) as hex, and the contract account ID.
	Nonce     string
	Recipient string
}

func (s *Service) terms(t domain.EscrowTerms) Terms {
	msg := t.Message(s.ContractID())
	nonce := domain.TermsNonce(msg)
	return Terms{Terms: t, Message: msg, Nonce: hex.EncodeToString(nonce[:]), Recipient: s.ContractID()}
}

// counterpartyTerms combines the creator's details with the other party's.
func (s *Service) counterpartyTerms(e domain.Escrow, in DetailsInput, deadlineMS uint64) (domain.EscrowTerms, error) {
	f, c, err := parseDetails(e.CreatorRole.Other(), in)
	if err != nil {
		return domain.EscrowTerms{}, err
	}
	if f == nil {
		f = e.Freelancer
	} else {
		c = e.Client
	}
	if f == nil || c == nil {
		return domain.EscrowTerms{}, fmt.Errorf("escrow %s: creator details missing", e.ID)
	}
	t := domain.NewEscrowTerms(e.ID, e.MilestoneAmounts(), *f, *c, e.FeeBPS, deadlineMS)
	if err := t.Validate(s.ContractID()); err != nil {
		return domain.EscrowTerms{}, err
	}
	return t, nil
}

func requireOpen(e domain.Escrow) error {
	switch e.Status {
	case domain.EscrowOpen:
		return nil
	case domain.EscrowPendingApproval:
		return domain.Conflict("escrow already has a submission waiting for the creator's approval")
	default:
		return domain.Conflict("escrow is %s, not Open", e.Status)
	}
}

// BuildTerms returns the final terms and the exact message the other party
// signs, from their details. It changes nothing (R-23).
func (s *Service) BuildTerms(ctx context.Context, id string, in DetailsInput) (Terms, error) {
	e, err := s.get(ctx, id)
	if err != nil {
		return Terms{}, err
	}
	if err := requireOpen(e); err != nil {
		return Terms{}, err
	}
	deadline := uint64(s.rt.now().Add(s.cfg.TermsTTL).UnixMilli())
	t, err := s.counterpartyTerms(e, in, deadline)
	if err != nil {
		return Terms{}, err
	}
	return s.terms(t), nil
}

// SubmitInput is the other party's details and their signature over the
// terms built from them.
type SubmitInput struct {
	Details    DetailsInput
	DeadlineMS uint64
	Signature  string
	// PublicKey is the NEAR key that signed, for NEP-413 signatures.
	PublicKey string
}

// SubmitResult is the stored submission and the submitter's guest token.
type SubmitResult struct {
	Escrow         domain.Escrow
	GuestToken     string
	GuestExpiresAt time.Time
}

// SubmitCounterparty verifies the other party's signature over the terms
// rebuilt from their details, stores the submission and moves the escrow to
// PendingApproval (R-23). The submitter gets a guest token (R-21). Their
// address comes from their signature, never from an account.
func (s *Service) SubmitCounterparty(ctx context.Context, id string, in SubmitInput) (SubmitResult, error) {
	e, err := s.get(ctx, id)
	if err != nil {
		return SubmitResult{}, err
	}
	if err := requireOpen(e); err != nil {
		return SubmitResult{}, err
	}
	now := s.rt.now()
	if in.DeadlineMS <= uint64(now.UnixMilli()) {
		return SubmitResult{}, domain.InvalidArgument("deadline_ms has passed; build the terms again")
	}
	if in.DeadlineMS > uint64(now.Add(s.cfg.TermsTTL+deadlineSkew).UnixMilli()) {
		return SubmitResult{}, domain.InvalidArgument("deadline_ms is later than the backend allows")
	}
	t, err := s.counterpartyTerms(e, in.Details, in.DeadlineMS)
	if err != nil {
		return SubmitResult{}, err
	}
	msg := t.Message(s.ContractID())

	sub := domain.Submission{
		Status:                domain.SubmissionPending,
		Terms:                 t,
		Message:               msg,
		CounterpartySignature: in.Signature,
	}
	if e.CreatorRole == domain.RoleClient {
		// The freelancer joins.
		if err := s.verifyFreelancerTerms(ctx, t, msg, in.PublicKey, in.Signature); err != nil {
			return SubmitResult{}, err
		}
		sub.Wallet = domain.Wallet{Scheme: domain.SchemeNEP413, Address: t.Freelancer}
		sub.CounterpartyPublicKey = domain.FormatNEARPublicKey(mustKey(in.PublicKey))
	} else {
		// The client joins with the approver wallet.
		if err := s.verifyApproverTerms(ctx, t.Approver, msg, in.Signature); err != nil {
			return SubmitResult{}, err
		}
		sub.Wallet = t.Approver.Wallet()
	}
	id2, err := uuid.NewRandomFromReader(s.rt.random)
	if err != nil {
		return SubmitResult{}, fmt.Errorf("escrow: generate submission id: %w", err)
	}
	sub.ID = id2.String()

	err = s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		e, err := s.lock(ctx, id)
		if err != nil {
			return err
		}
		if err := requireOpen(e); err != nil {
			return err
		}
		err = s.Repo.InsertSubmission(ctx, id, sub)
		if errors.Is(err, ErrActiveSubmission) {
			return domain.Conflict("escrow already has a submission waiting for the creator's approval")
		}
		if err != nil {
			return fmt.Errorf("escrow: insert submission: %w", err)
		}
		if err := s.Repo.SetStatus(ctx, id, domain.EscrowPendingApproval); err != nil {
			return fmt.Errorf("escrow: set status: %w", err)
		}
		e.Submission = &sub
		return s.Repo.ReplaceParties(ctx, id, e.Parties())
	})
	if err != nil {
		return SubmitResult{}, err
	}
	s.rt.log.InfoContext(ctx, "escrow submission received",
		slog.String("escrow_id", id), slog.String("scheme", string(sub.Wallet.Scheme)))

	token, expires, err := s.Guests.Issue(ctx, id, sub.Wallet)
	if err != nil {
		return SubmitResult{}, fmt.Errorf("escrow: issue guest token: %w", err)
	}
	e, err = s.get(ctx, id)
	if err != nil {
		return SubmitResult{}, err
	}
	return SubmitResult{Escrow: e, GuestToken: token, GuestExpiresAt: expires}, nil
}

// mustKey decodes a public key already validated by the signature check.
func mustKey(s string) []byte {
	k, _ := domain.ParseNEARPublicKey(s)
	return k
}

// pendingForCreator returns the escrow if accountID created it and it waits
// for the creator's decision.
func (s *Service) pendingForCreator(ctx context.Context, accountID, id string) (domain.Escrow, error) {
	e, err := s.get(ctx, id)
	if err != nil {
		return domain.Escrow{}, err
	}
	if e.CreatorAccountID != accountID {
		return domain.Escrow{}, domain.Forbidden("only the creator can approve or reject the submission")
	}
	if e.Status != domain.EscrowPendingApproval || e.Submission == nil || e.Submission.Status != domain.SubmissionPending {
		return domain.Escrow{}, domain.Conflict("escrow has no submission waiting for approval (status %s)", e.Status)
	}
	return e, nil
}

// ApproveInput is the creator's signature over the submitted terms.
type ApproveInput struct {
	Signature string
	// PublicKey is the NEAR key that signed, when the creator is the
	// freelancer.
	PublicKey string
}

// Approve verifies the creator's signature over the same terms and marks
// the escrow Approved; a job then creates it on-chain (R-23).
func (s *Service) Approve(ctx context.Context, accountID, id string, in ApproveInput) (domain.Escrow, error) {
	e, err := s.pendingForCreator(ctx, accountID, id)
	if err != nil {
		return domain.Escrow{}, err
	}
	sub := *e.Submission
	if s.nowMS() >= sub.Terms.DeadlineMS {
		if err := s.ExpireSubmission(ctx, id); err != nil {
			return domain.Escrow{}, err
		}
		return domain.Escrow{}, domain.Conflict("the submission expired; the escrow is Open again")
	}

	t := sub.Terms
	if e.CreatorRole == domain.RoleFreelancer {
		if err := s.verifyFreelancerTerms(ctx, t, sub.Message, in.PublicKey, in.Signature); err != nil {
			return domain.Escrow{}, err
		}
		sub.CreatorPublicKey = domain.FormatNEARPublicKey(mustKey(in.PublicKey))
	} else if err := s.verifyApproverTerms(ctx, t.Approver, sub.Message, in.Signature); err != nil {
		return domain.Escrow{}, err
	}
	sub.CreatorSignature = in.Signature
	sub.Status = domain.SubmissionApproved

	err = s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		locked, err := s.lock(ctx, id)
		if err != nil {
			return err
		}
		if locked.Status != domain.EscrowPendingApproval || locked.Submission == nil || locked.Submission.ID != sub.ID {
			return domain.Conflict("the submission changed; reload the escrow")
		}
		if err := s.Repo.UpdateSubmission(ctx, sub); err != nil {
			return fmt.Errorf("escrow: update submission: %w", err)
		}
		return s.Repo.SetStatus(ctx, id, domain.EscrowApproved)
	})
	if err != nil {
		return domain.Escrow{}, err
	}
	s.rt.log.InfoContext(ctx, "escrow approved by creator", slog.String("escrow_id", id))
	return s.get(ctx, id)
}

// Reject discards the submission and returns the escrow to Open (R-23).
func (s *Service) Reject(ctx context.Context, accountID, id string) (domain.Escrow, error) {
	if _, err := s.pendingForCreator(ctx, accountID, id); err != nil {
		return domain.Escrow{}, err
	}
	err := s.reopen(ctx, id, domain.SubmissionRejected, "", func(e domain.Escrow) bool {
		return e.Status == domain.EscrowPendingApproval
	})
	if err != nil {
		return domain.Escrow{}, err
	}
	s.rt.log.InfoContext(ctx, "escrow submission rejected", slog.String("escrow_id", id))
	return s.get(ctx, id)
}

// ExpireSubmission returns an escrow to Open if its pending submission's
// deadline has passed.
func (s *Service) ExpireSubmission(ctx context.Context, id string) error {
	now := s.nowMS()
	return s.reopen(ctx, id, domain.SubmissionExpired, "", func(e domain.Escrow) bool {
		return e.Status == domain.EscrowPendingApproval && e.Submission != nil && now >= e.Submission.Terms.DeadlineMS
	})
}

// errUnchanged stops a transaction whose escrow no longer needs the change.
var errUnchanged = errors.New("escrow: unchanged")

// reopen ends the active submission with status and returns the escrow to
// Open, if cond still holds for the locked escrow.
func (s *Service) reopen(ctx context.Context, id string, status domain.SubmissionStatus, reason string, cond func(domain.Escrow) bool) error {
	err := s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		e, err := s.lock(ctx, id)
		if err != nil {
			return err
		}
		if !cond(e) || e.Submission == nil {
			return errUnchanged
		}
		sub := *e.Submission
		sub.Status, sub.FailureReason = status, reason
		if err := s.Repo.UpdateSubmission(ctx, sub); err != nil {
			return fmt.Errorf("escrow: update submission: %w", err)
		}
		if err := s.Repo.SetStatus(ctx, id, domain.EscrowOpen); err != nil {
			return fmt.Errorf("escrow: set status: %w", err)
		}
		e.Submission = nil
		return s.Repo.ReplaceParties(ctx, id, e.Parties())
	})
	if errors.Is(err, errUnchanged) {
		return nil
	}
	if err == nil {
		s.rt.log.InfoContext(ctx, "escrow reopened", slog.String("escrow_id", id), slog.String("submission", string(status)))
	}
	return err
}

// cancelOffChain cancels an escrow that is not on-chain; only its creator
// can, and nothing is sent to the contract (R-23).
func (s *Service) cancelOffChain(ctx context.Context, accountID, id string) (domain.Escrow, error) {
	err := s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		e, err := s.lock(ctx, id)
		if err != nil {
			return err
		}
		if e.CreatorAccountID != accountID {
			return domain.Forbidden("only the creator can cancel an escrow that is not on-chain")
		}
		if e.Status != domain.EscrowOpen && e.Status != domain.EscrowPendingApproval {
			return domain.Conflict("escrow is %s and can no longer be cancelled by the creator", e.Status)
		}
		if e.Submission != nil {
			sub := *e.Submission
			sub.Status = domain.SubmissionCancelled
			if err := s.Repo.UpdateSubmission(ctx, sub); err != nil {
				return fmt.Errorf("escrow: update submission: %w", err)
			}
		}
		return s.Repo.SetStatus(ctx, id, domain.EscrowCancelled)
	})
	if err != nil {
		return domain.Escrow{}, err
	}
	s.rt.log.InfoContext(ctx, "escrow cancelled before going on-chain", slog.String("escrow_id", id))
	return s.get(ctx, id)
}
