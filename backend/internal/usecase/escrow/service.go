// Package escrow implements escrows from proposal to payout: creating and
// joining an escrow, the creator's approval, creating it on-chain, relaying
// signed actions to the contract, and the jobs that keep it moving
// (spec.md R-07, R-09, R-16, R-20, R-21, R-23).
package escrow

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/harundarat/vecrow/backend/internal/domain"
	"github.com/harundarat/vecrow/backend/internal/usecase"
)

// Config holds the escrow settings.
type Config struct {
	// FeeBPS is the contract's fee; terms carry it and the contract checks it.
	FeeBPS uint16
	// TermsTTL is how long signed terms stay valid: their deadline is the
	// time they are built plus TermsTTL.
	TermsTTL time.Duration
	// AppURL is the frontend's base URL; links are AppURL/escrows/{id}.
	AppURL string
	// RelayerMaxGas caps the gas of a delegate action's function call.
	RelayerMaxGas uint64
	// ExpiryGrace is waited after a deadline before an approved escrow that
	// is not on-chain returns to Open, so a create_escrow sent just before
	// the deadline is seen first.
	ExpiryGrace time.Duration
}

// Deps are the ports the service uses.
type Deps struct {
	Repo     Repository
	Tx       usecase.Transactor
	Wallets  WalletOwners
	Keys     AccessKeys
	Contract Contract
	Storage  Storage
	Guests   GuestTokens
}

// Service implements the escrow use cases.
type Service struct {
	Deps
	cfg Config
	rt  runtime
}

// runtime holds the sources of randomness and time, replaceable in tests,
// and the logger for business events.
type runtime struct {
	random io.Reader
	now    func() time.Time
	log    *slog.Logger
}

// Option configures the service.
type Option func(*runtime)

func WithRandom(r io.Reader) Option { return func(rt *runtime) { rt.random = r } }

func WithClock(now func() time.Time) Option { return func(rt *runtime) { rt.now = now } }

// WithLogger sets the logger for business events. The default discards.
func WithLogger(l *slog.Logger) Option { return func(rt *runtime) { rt.log = l } }

func NewService(d Deps, cfg Config, opts ...Option) *Service {
	rt := runtime{random: rand.Reader, now: time.Now, log: slog.New(slog.DiscardHandler)}
	for _, opt := range opts {
		opt(&rt)
	}
	cfg.AppURL = strings.TrimRight(cfg.AppURL, "/")
	return &Service{Deps: d, cfg: cfg, rt: rt}
}

// ContractID is the escrow contract the terms are signed for.
func (s *Service) ContractID() string { return s.Contract.ID() }

// Link is the URL the creator shares with the other party.
func (s *Service) Link(id string) string { return s.cfg.AppURL + "/escrows/" + id }

func (s *Service) nowMS() uint64 { return uint64(s.rt.now().UnixMilli()) }

// FreelancerInput is the freelancer's own details.
type FreelancerInput struct {
	Account string
	Split   []domain.SplitShare
}

// ApproverInput names the approver wallet: account_id and public_key for
// nep413, address for eip191 and ed25519.
type ApproverInput struct {
	Scheme    string
	AccountID string
	PublicKey string
	Address   string
}

// ClientInput is the client's own details.
type ClientInput struct {
	Approver      ApproverInput
	RefundAccount string
}

// DetailsInput carries the details of one role; only that role's are set.
type DetailsInput struct {
	Freelancer *FreelancerInput
	Client     *ClientInput
}

// parseDetails validates the details of role, which must be the only ones
// given.
func parseDetails(role domain.Role, in DetailsInput) (*domain.FreelancerDetails, *domain.ClientDetails, error) {
	switch role {
	case domain.RoleFreelancer:
		if in.Freelancer == nil || in.Client != nil {
			return nil, nil, domain.InvalidArgument("as freelancer, give freelancer and split, and no approver or refund_account")
		}
		f := domain.FreelancerDetails{Account: in.Freelancer.Account, Split: in.Freelancer.Split}
		if err := f.Validate(); err != nil {
			return nil, nil, err
		}
		return &f, nil, nil
	default:
		if in.Client == nil || in.Freelancer != nil {
			return nil, nil, domain.InvalidArgument("as client, give approver and refund_account, and no freelancer or split")
		}
		scheme, err := domain.ParseWalletScheme(in.Client.Approver.Scheme)
		if err != nil {
			return nil, nil, err
		}
		a := in.Client.Approver
		approver, err := domain.NewApprover(scheme, a.AccountID, a.PublicKey, a.Address)
		if err != nil {
			return nil, nil, err
		}
		c := domain.ClientDetails{Approver: approver, RefundAccount: in.Client.RefundAccount}
		if err := c.Validate(); err != nil {
			return nil, nil, err
		}
		return nil, &c, nil
	}
}

// checkOneSide applies the rules that involve only one party's details and
// the contract, so the creator learns of them before sharing the link.
func checkOneSide(contractID string, f *domain.FreelancerDetails, c *domain.ClientDetails) error {
	if f != nil {
		if f.Account == contractID {
			return domain.InvalidArgument("the escrow contract cannot be the freelancer")
		}
		for _, sh := range f.Split {
			if sh.AccountID == contractID {
				return domain.InvalidArgument("the escrow contract cannot be a split recipient")
			}
		}
	}
	if c != nil && c.RefundAccount == contractID {
		return domain.InvalidArgument("refund_account must not be the escrow contract")
	}
	return nil
}

// CreateInput is a new escrow proposal.
type CreateInput struct {
	AccountID  string
	Role       string
	Milestones []string
	Details    DetailsInput
}

// Create stores a new escrow as Open (R-23). The creator's own address must
// be a wallet of their account.
func (s *Service) Create(ctx context.Context, in CreateInput) (domain.Escrow, error) {
	role, err := domain.ParseRole(in.Role)
	if err != nil {
		return domain.Escrow{}, err
	}
	if _, err := domain.ValidateMilestones(in.Milestones); err != nil {
		return domain.Escrow{}, err
	}
	f, c, err := parseDetails(role, in.Details)
	if err != nil {
		return domain.Escrow{}, err
	}
	if err := checkOneSide(s.ContractID(), f, c); err != nil {
		return domain.Escrow{}, err
	}

	id, err := uuid.NewRandomFromReader(s.rt.random)
	if err != nil {
		return domain.Escrow{}, fmt.Errorf("escrow: generate id: %w", err)
	}
	e := domain.Escrow{
		ID:               id.String(),
		CreatorAccountID: in.AccountID,
		CreatorRole:      role,
		Status:           domain.EscrowOpen,
		FeeBPS:           s.cfg.FeeBPS,
		Freelancer:       f,
		Client:           c,
	}
	for _, m := range in.Milestones {
		e.Milestones = append(e.Milestones, domain.Milestone{Amount: m, Status: domain.MilestonePending})
	}
	wallet := e.CreatorWallet()
	owns, err := s.Wallets.IsWalletOwner(ctx, in.AccountID, wallet)
	if err != nil {
		return domain.Escrow{}, fmt.Errorf("escrow: check creator wallet: %w", err)
	}
	if !owns {
		if role == domain.RoleFreelancer {
			return domain.Escrow{}, domain.Forbidden("freelancer %s is not a NEAR wallet of your account", wallet.Address)
		}
		return domain.Escrow{}, domain.Forbidden("approver %s is not a wallet of your account", wallet.Address)
	}

	err = s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.Repo.Create(ctx, e); err != nil {
			return fmt.Errorf("escrow: create: %w", err)
		}
		return s.Repo.ReplaceParties(ctx, e.ID, e.Parties())
	})
	if err != nil {
		return domain.Escrow{}, err
	}
	s.rt.log.InfoContext(ctx, "escrow created",
		slog.String("escrow_id", e.ID), slog.String("account_id", in.AccountID), slog.String("role", string(role)))
	return s.get(ctx, e.ID)
}

// View is an escrow with what GET /escrows/:id shows besides it.
type View struct {
	Escrow        domain.Escrow
	Transactions  []domain.Transaction
	RevisionNotes []domain.RevisionNote
}

// Get returns an escrow for anyone with the link. A guest token, if given,
// must be for this escrow. Escrows on-chain are refreshed from the contract
// first; if that fails, the last snapshot is shown.
func (s *Service) Get(ctx context.Context, id string, req domain.Requester) (View, error) {
	if req.Guest != nil && req.Guest.EscrowID != id {
		return View{}, domain.Unauthorized("guest token is for another escrow")
	}
	e, err := s.get(ctx, id)
	if err != nil {
		return View{}, err
	}
	if e.OnChain() {
		if err := s.Sync(ctx, id); err != nil {
			s.rt.log.WarnContext(ctx, "escrow refresh failed, showing last snapshot",
				slog.String("escrow_id", id), slog.String("error", err.Error()))
		} else if e, err = s.get(ctx, id); err != nil {
			return View{}, err
		}
	}
	txs, err := s.Repo.ListTransactions(ctx, id)
	if err != nil {
		return View{}, fmt.Errorf("escrow: list transactions: %w", err)
	}
	notes, err := s.Repo.ListRevisionNotes(ctx, id)
	if err != nil {
		return View{}, fmt.Errorf("escrow: list revision notes: %w", err)
	}
	return View{Escrow: e, Transactions: txs, RevisionNotes: notes}, nil
}

// Escrow returns the stored escrow, without refreshing it.
func (s *Service) Escrow(ctx context.Context, id string) (domain.Escrow, error) {
	return s.get(ctx, id)
}

func (s *Service) get(ctx context.Context, id string) (domain.Escrow, error) {
	if domain.ValidateEscrowID(id) != nil {
		return domain.Escrow{}, domain.NotFound("escrow not found")
	}
	e, err := s.Repo.Get(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return domain.Escrow{}, domain.NotFound("escrow not found")
	}
	if err != nil {
		return domain.Escrow{}, fmt.Errorf("escrow: get: %w", err)
	}
	return e, nil
}

// lock returns the escrow locked until the transaction ends.
func (s *Service) lock(ctx context.Context, id string) (domain.Escrow, error) {
	e, err := s.Repo.Lock(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return domain.Escrow{}, domain.NotFound("escrow not found")
	}
	if err != nil {
		return domain.Escrow{}, fmt.Errorf("escrow: lock: %w", err)
	}
	return e, nil
}

// ListMine returns the account's escrows by role and status (R-16).
func (s *Service) ListMine(ctx context.Context, accountID, role, status string) ([]AccountEscrow, error) {
	var f ListFilter
	if role != "" {
		switch r := domain.Role(role); r {
		case domain.RoleFreelancer, domain.RoleClient, domain.RoleMember:
			f.Role = r
		default:
			return nil, domain.InvalidArgument("role must be freelancer, client or member")
		}
	}
	if status != "" {
		st, err := domain.ParseEscrowStatus(status)
		if err != nil {
			return nil, err
		}
		f.Status = st
	}
	out, err := s.Repo.ListForAccount(ctx, accountID, f)
	if err != nil {
		return nil, fmt.Errorf("escrow: list for account: %w", err)
	}
	return out, nil
}
