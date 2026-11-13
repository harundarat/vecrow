package escrow

import (
	"context"
	"errors"
	"time"

	"github.com/harundarat/vecrow/backend/internal/domain"
	"github.com/harundarat/vecrow/backend/internal/neartx"
)

// Errors returned by the ports. The use cases translate them into
// client-facing domain errors.
var (
	ErrNotFound = errors.New("escrow: not found")
	// ErrActiveSubmission means the escrow already has an active submission.
	ErrActiveSubmission = errors.New("escrow: active submission exists")
)

// Repository persists escrows. Calls made with a ctx from
// usecase.Transactor.WithinTx join that transaction.
type Repository interface {
	// Create inserts the escrow with its milestones.
	Create(ctx context.Context, e domain.Escrow) error
	// Get returns the escrow with its milestones, active submission and
	// pending payouts, or ErrNotFound.
	Get(ctx context.Context, id string) (domain.Escrow, error)
	// Lock is Get that also locks the escrow until the transaction ends.
	Lock(ctx context.Context, id string) (domain.Escrow, error)
	SetStatus(ctx context.Context, id string, status domain.EscrowStatus) error
	// InsertSubmission stores a pending submission, or returns
	// ErrActiveSubmission without failing the transaction.
	InsertSubmission(ctx context.Context, escrowID string, s domain.Submission) error
	// UpdateSubmission stores the status, creator signature and failure
	// reason of s.
	UpdateSubmission(ctx context.Context, s domain.Submission) error
	// ReplaceParties sets the wallets with a role in the escrow.
	ReplaceParties(ctx context.Context, escrowID string, parties []domain.Party) error
	// SaveChain stores a get_escrow snapshot (status, milestones, balance,
	// pending payouts). A snapshot read at a lower block height than the
	// stored one is ignored; it reports whether it was stored.
	SaveChain(ctx context.Context, c domain.ChainEscrow) (bool, error)
	// SaveTransaction stores a transaction with its events. Saving the same
	// hash again does nothing.
	SaveTransaction(ctx context.Context, tx domain.Transaction) error
	ListTransactions(ctx context.Context, escrowID string) ([]domain.Transaction, error)
	AddRevisionNote(ctx context.Context, escrowID string, n domain.RevisionNote) error
	ListRevisionNotes(ctx context.Context, escrowID string) ([]domain.RevisionNote, error)
	// SetInvitation stores the escrow's invitation, replacing an earlier
	// one; nil removes it. Get returns it in Escrow.Invitation.
	SetInvitation(ctx context.Context, escrowID string, inv *domain.Invitation) error
	// ListForAccount returns the escrows in which the account has a role
	// (as creator, through a linked wallet, or by invitation), newest first.
	ListForAccount(ctx context.Context, accountID string, f ListFilter) ([]AccountEscrow, error)

	// Due work for the jobs (architecture.md §7.3).
	ListApproved(ctx context.Context, limit int) ([]string, error)
	ListExpiredSubmissions(ctx context.Context, nowMS uint64, limit int) ([]string, error)
	ListDueReleases(ctx context.Context, nowMS uint64, limit int) ([]MilestoneRef, error)
	ListPendingPayouts(ctx context.Context, limit int) ([]PayoutRef, error)
	// ListToSync returns escrows on-chain and not settled (Created, Funded,
	// or with pending payouts) last synced before olderThan.
	ListToSync(ctx context.Context, olderThan time.Time, limit int) ([]string, error)
}

// ListFilter narrows ListForAccount. Empty fields match everything.
type ListFilter struct {
	Role   domain.Role
	Status domain.EscrowStatus
}

// AccountEscrow is an escrow with the account's roles in it.
type AccountEscrow struct {
	Escrow domain.Escrow
	Roles  []domain.Role
	// Invited reports that the account was invited to the escrow (R-15);
	// the invited role is among Roles.
	Invited bool
}

// MilestoneRef names a milestone of an escrow.
type MilestoneRef struct {
	EscrowID  string `json:"escrow_id"`
	Milestone uint32 `json:"milestone"`
}

// PayoutRef names a pending payout of an escrow.
type PayoutRef struct {
	EscrowID  string `json:"escrow_id"`
	AccountID string `json:"account_id"`
}

// WalletOwners tells whether a wallet is linked to an account.
type WalletOwners interface {
	IsWalletOwner(ctx context.Context, accountID string, w domain.Wallet) (bool, error)
}

// Invitees finds the accounts invitations name (R-15).
type Invitees interface {
	// AccountByUsername returns the ID of the account with username, or ""
	// when there is none.
	AccountByUsername(ctx context.Context, username string) (string, error)
	// InvitedAccount returns the account inv names now: the account of an
	// invitation by username, the account whose verified email it is, or
	// the account its wallet is linked to; "" when there is none.
	InvitedAccount(ctx context.Context, inv domain.Invitation) (string, error)
}

// AccessKeys looks up access keys on the NEAR chain.
type AccessKeys interface {
	Status(ctx context.Context, accountID, publicKey string) (domain.AccessKeyStatus, error)
}

// GuestTokens issues guest tokens (R-21).
type GuestTokens interface {
	Issue(ctx context.Context, escrowID string, w domain.Wallet) (token string, expiresAt time.Time, err error)
}

// ApproverAction is the approver's signed authorization of an action.
type ApproverAction struct {
	// Nonce is 64 lowercase hex characters.
	Nonce      string
	DeadlineMS uint64
	Signature  string
}

// Contract is the escrow contract. Methods that send a transaction return
// it with Succeeded false when the contract rejected the call, and an error
// only when the outcome is unknown or the transaction was not sent; the
// relayer pays the gas.
type Contract interface {
	// ID is the contract account ID.
	ID() string
	// GetEscrow returns the escrow, or nil if the contract has none.
	GetEscrow(ctx context.Context, escrowID string) (*domain.ChainEscrow, error)
	CreateEscrow(ctx context.Context, t domain.EscrowTerms, f domain.FreelancerSignature, clientSignature string) (domain.Transaction, error)
	ApproveMilestone(ctx context.Context, escrowID string, milestone uint32, a ApproverAction) (domain.Transaction, error)
	RequestRevision(ctx context.Context, escrowID string, milestone uint32, a ApproverAction) (domain.Transaction, error)
	// Cancel cancels with the approver's signature.
	Cancel(ctx context.Context, escrowID string, a ApproverAction) (domain.Transaction, error)
	AutoRelease(ctx context.Context, escrowID string, milestone uint32) (domain.Transaction, error)
	RetryPayout(ctx context.Context, escrowID, accountID string) (domain.Transaction, error)
	// Relay submits a signed delegate action the use case has checked.
	Relay(ctx context.Context, escrowID string, a neartx.SignedDelegateAction) (domain.Transaction, error)
}

// Storage registers accounts on the USDC contract (NEP-145), paid by the
// backend's storage account (R-20).
type Storage interface {
	IsRegistered(ctx context.Context, accountID string) (bool, error)
	// Register pays storage_deposit with registration_only for accountID.
	Register(ctx context.Context, escrowID, accountID string) (domain.Transaction, error)
}
