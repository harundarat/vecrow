package escrow

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/harundarat/vecrow/backend/internal/domain"
)

// Queue topics of the escrow jobs (architecture.md §7.3).
const (
	TopicCreate      = "escrow.create"
	TopicSync        = "escrow.sync"
	TopicAutoRelease = "escrow.auto_release"
	TopicRetryPayout = "escrow.retry_payout"
)

// EscrowRef is the payload of the create and sync jobs.
type EscrowRef struct {
	EscrowID string `json:"escrow_id"`
}

// ErrNotDue means a job ran before its work was due; it is retried later.
var ErrNotDue = errors.New("escrow: not due yet")

// CreateOnChain creates an approved escrow on-chain (R-23, R-20, R-09): it
// registers storage for the split recipients and the refund account, then
// sends create_escrow with both signatures through the relayer. It is
// idempotent: an escrow the contract already has is marked Created, and
// terms the contract rejects return the escrow to Open.
func (s *Service) CreateOnChain(ctx context.Context, id string) error {
	e, err := s.get(ctx, id)
	if err != nil {
		return err
	}
	if e.Status != domain.EscrowApproved {
		return nil // done, or no longer wanted
	}
	t, ok := e.Terms()
	if !ok {
		return fmt.Errorf("escrow %s: approved without terms", id)
	}

	chain, err := s.Contract.GetEscrow(ctx, id)
	if err != nil {
		return fmt.Errorf("escrow: get_escrow: %w", err)
	}
	if chain != nil {
		return s.markCreated(ctx, e, t, chain)
	}

	now := s.nowMS()
	if now > t.DeadlineMS {
		// create_escrow would be rejected now. Wait out the grace period so a
		// call sent before the deadline shows up in get_escrow first.
		if now <= t.DeadlineMS+uint64(s.cfg.ExpiryGrace.Milliseconds()) {
			return ErrNotDue
		}
		return s.reopen(ctx, id, domain.SubmissionExpired, "the terms expired before the escrow was created on-chain",
			func(e domain.Escrow) bool { return e.Status == domain.EscrowApproved })
	}

	// The freelancer's key may have been removed since it signed.
	f, _ := e.Submission.Signatures(e.CreatorRole)
	pub, err := domain.ParseNEARPublicKey(f.PublicKey)
	if err != nil {
		return fmt.Errorf("escrow %s: stored freelancer key: %w", id, err)
	}
	if err := s.checkFullAccess(ctx, t.Freelancer, f.PublicKey, pub); err != nil {
		var de *domain.Error
		if errors.As(err, &de) {
			return s.reopen(ctx, id, domain.SubmissionFailed, de.Message,
				func(e domain.Escrow) bool { return e.Status == domain.EscrowApproved })
		}
		return err
	}

	if err := s.ensureStorage(ctx, id, storageAccounts(t)); err != nil {
		return err
	}

	fsig, csig := e.Submission.Signatures(e.CreatorRole)
	tx, err := s.Contract.CreateEscrow(ctx, t, fsig, csig)
	if err != nil {
		return fmt.Errorf("escrow: create_escrow: %w", err)
	}
	if err := s.record(ctx, id, tx); err != nil {
		return err
	}
	if !tx.Succeeded && !strings.Contains(tx.Error, "already exists") {
		s.rt.log.WarnContext(ctx, "contract rejected create_escrow",
			slog.String("escrow_id", id), slog.String("tx_hash", tx.Hash), slog.String("reason", tx.Error))
		return s.reopen(ctx, id, domain.SubmissionFailed, tx.Error,
			func(e domain.Escrow) bool { return e.Status == domain.EscrowApproved })
	}
	chain, err = s.Contract.GetEscrow(ctx, id)
	if err != nil {
		return fmt.Errorf("escrow: get_escrow: %w", err)
	}
	if chain == nil {
		return fmt.Errorf("escrow %s: create_escrow succeeded but get_escrow has no escrow", id)
	}
	return s.markCreated(ctx, e, t, chain)
}

// storageAccounts are the accounts that receive payouts and must be
// registered on the USDC contract: the split recipients and the refund
// account. The treasury and the contract are registered at deploy.
func storageAccounts(t domain.EscrowTerms) []string {
	var out []string
	seen := map[string]bool{}
	for _, a := range append(splitAccounts(t), t.RefundAccount) {
		if !seen[a] {
			seen[a] = true
			out = append(out, a)
		}
	}
	return out
}

func splitAccounts(t domain.EscrowTerms) []string {
	out := make([]string, len(t.Split))
	for i, s := range t.Split {
		out[i] = s.AccountID
	}
	return out
}

// ensureStorage registers each account that is not registered yet.
func (s *Service) ensureStorage(ctx context.Context, id string, accounts []string) error {
	for _, a := range accounts {
		ok, err := s.Storage.IsRegistered(ctx, a)
		if err != nil {
			return fmt.Errorf("escrow: storage_balance_of %s: %w", a, err)
		}
		if ok {
			continue
		}
		tx, err := s.Storage.Register(ctx, id, a)
		if err != nil {
			return fmt.Errorf("escrow: storage_deposit %s: %w", a, err)
		}
		if err := s.record(ctx, id, tx); err != nil {
			return err
		}
		if !tx.Succeeded {
			return fmt.Errorf("escrow: storage_deposit %s failed: %s", a, tx.Error)
		}
		s.rt.log.InfoContext(ctx, "storage registered",
			slog.String("escrow_id", id), slog.String("account_id", a), slog.String("tx_hash", tx.Hash))
	}
	return nil
}

// markCreated marks the escrow Created if the contract's escrow has the
// signed terms, and stores its state.
func (s *Service) markCreated(ctx context.Context, e domain.Escrow, t domain.EscrowTerms, chain *domain.ChainEscrow) error {
	if !chain.MatchesTerms(t) {
		// Escrow IDs are random UUIDs, so this means another escrow took the
		// ID: these terms can never be created.
		s.rt.log.ErrorContext(ctx, "escrow ID used on-chain with other terms", slog.String("escrow_id", e.ID))
		return s.Tx.WithinTx(ctx, func(ctx context.Context) error {
			locked, err := s.lock(ctx, e.ID)
			if err != nil || locked.Status != domain.EscrowApproved || locked.Submission == nil {
				return err
			}
			sub := *locked.Submission
			sub.Status, sub.FailureReason = domain.SubmissionFailed, "the escrow ID is used on-chain by other terms"
			if err := s.Repo.UpdateSubmission(ctx, sub); err != nil {
				return err
			}
			return s.Repo.SetStatus(ctx, e.ID, domain.EscrowCancelled)
		})
	}
	return s.applyChain(ctx, chain)
}

// Sync reads the escrow from the contract and stores its state. Escrows the
// contract does not have are left alone.
func (s *Service) Sync(ctx context.Context, id string) error {
	chain, err := s.Contract.GetEscrow(ctx, id)
	if err != nil {
		return fmt.Errorf("escrow: get_escrow: %w", err)
	}
	if chain == nil {
		return nil
	}
	return s.applyChain(ctx, chain)
}

// applyChain stores a snapshot of the contract's escrow. An approved
// submission whose terms the contract has becomes created.
func (s *Service) applyChain(ctx context.Context, chain *domain.ChainEscrow) error {
	return s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		e, err := s.lock(ctx, chain.EscrowID)
		if err != nil {
			return err
		}
		t, ok := e.Terms()
		if !ok || !chain.MatchesTerms(t) {
			s.rt.log.WarnContext(ctx, "contract escrow does not match the approved terms, not stored",
				slog.String("escrow_id", e.ID))
			return nil
		}
		if e.Submission.Status == domain.SubmissionApproved {
			sub := *e.Submission
			sub.Status = domain.SubmissionCreated
			if err := s.Repo.UpdateSubmission(ctx, sub); err != nil {
				return fmt.Errorf("escrow: update submission: %w", err)
			}
			s.rt.log.InfoContext(ctx, "escrow created on-chain", slog.String("escrow_id", e.ID))
		}
		if _, err := s.Repo.SaveChain(ctx, *chain); err != nil {
			return fmt.Errorf("escrow: save chain state: %w", err)
		}
		return nil
	})
}

// AutoRelease calls auto_release for a milestone whose review window ended
// (R-07). A call the contract rejects, because the milestone was approved or
// changed meanwhile, ends the job.
func (s *Service) AutoRelease(ctx context.Context, id string, milestone uint32) error {
	if err := s.Sync(ctx, id); err != nil {
		return err
	}
	e, err := s.get(ctx, id)
	if err != nil {
		return err
	}
	if int(milestone) >= len(e.Milestones) || !s.releaseDue(e, e.Milestones[milestone]) {
		return nil
	}
	tx, err := s.Contract.AutoRelease(ctx, id, milestone)
	if err != nil {
		return fmt.Errorf("escrow: auto_release: %w", err)
	}
	return s.afterJobTx(ctx, id, "auto_release", tx)
}

// releaseDue reports whether a milestone can be auto-released now.
func (s *Service) releaseDue(e domain.Escrow, m domain.Milestone) bool {
	if e.Status != domain.EscrowFunded || m.Status != domain.MilestoneSubmitted || m.SubmittedAtMS == nil ||
		e.ReviewWindowSec == nil {
		return false
	}
	return s.nowMS() >= *m.SubmittedAtMS+*e.ReviewWindowSec*1000
}

// RetryPayout registers the account if needed, then calls retry_payout for
// its pending payout (R-20). The contract zeroes the pending amount before
// sending, so a repeated call never pays twice.
func (s *Service) RetryPayout(ctx context.Context, id, account string) error {
	if err := s.Sync(ctx, id); err != nil {
		return err
	}
	e, err := s.get(ctx, id)
	if err != nil {
		return err
	}
	pending := false
	for _, p := range e.PendingPayouts {
		pending = pending || p.AccountID == account
	}
	if !pending {
		return nil
	}
	if err := s.ensureStorage(ctx, id, []string{account}); err != nil {
		return err
	}
	tx, err := s.Contract.RetryPayout(ctx, id, account)
	if err != nil {
		return fmt.Errorf("escrow: retry_payout: %w", err)
	}
	return s.afterJobTx(ctx, id, "retry_payout", tx)
}

// afterJobTx records a job's transaction and refreshes the escrow.
func (s *Service) afterJobTx(ctx context.Context, id, method string, tx domain.Transaction) error {
	if err := s.record(ctx, id, tx); err != nil {
		return err
	}
	if tx.Succeeded {
		s.rt.log.InfoContext(ctx, "escrow job transaction succeeded", slog.String("escrow_id", id),
			slog.String("method", method), slog.String("tx_hash", tx.Hash), slog.Any("events", eventNames(tx)))
	} else {
		s.rt.log.WarnContext(ctx, "contract rejected escrow job call", slog.String("escrow_id", id),
			slog.String("method", method), slog.String("tx_hash", tx.Hash), slog.String("reason", tx.Error))
	}
	return s.Sync(ctx, id)
}

// Due lists the work the jobs have to do, for the scheduler.
type Due struct {
	Create      []string
	Sync        []string
	AutoRelease []MilestoneRef
	RetryPayout []PayoutRef
}

// FindDue lists due work, at most limit items of each kind, and returns
// expired pending submissions to Open on the way.
func (s *Service) FindDue(ctx context.Context, syncInterval time.Duration, limit int) (Due, error) {
	expired, err := s.Repo.ListExpiredSubmissions(ctx, s.nowMS(), limit)
	if err != nil {
		return Due{}, fmt.Errorf("escrow: list expired submissions: %w", err)
	}
	for _, id := range expired {
		if err := s.ExpireSubmission(ctx, id); err != nil {
			return Due{}, err
		}
	}
	var d Due
	if d.Create, err = s.Repo.ListApproved(ctx, limit); err != nil {
		return Due{}, fmt.Errorf("escrow: list approved: %w", err)
	}
	if d.Sync, err = s.Repo.ListToSync(ctx, s.rt.now().Add(-syncInterval), limit); err != nil {
		return Due{}, fmt.Errorf("escrow: list to sync: %w", err)
	}
	if d.AutoRelease, err = s.Repo.ListDueReleases(ctx, s.nowMS(), limit); err != nil {
		return Due{}, fmt.Errorf("escrow: list due releases: %w", err)
	}
	if d.RetryPayout, err = s.Repo.ListPendingPayouts(ctx, limit); err != nil {
		return Due{}, fmt.Errorf("escrow: list pending payouts: %w", err)
	}
	return d, nil
}
