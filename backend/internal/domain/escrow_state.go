package domain

import (
	"strings"
	"time"
)

// SubmissionStatus is the status of the other party's signed submission.
type SubmissionStatus string

const (
	// SubmissionPending waits for the creator's approval.
	SubmissionPending SubmissionStatus = "pending"
	// SubmissionApproved has both signatures; create_escrow is queued.
	SubmissionApproved SubmissionStatus = "approved"
	// SubmissionCreated is on-chain: its terms are the escrow's terms.
	SubmissionCreated   SubmissionStatus = "created"
	SubmissionRejected  SubmissionStatus = "rejected"
	SubmissionExpired   SubmissionStatus = "expired"
	SubmissionFailed    SubmissionStatus = "failed"
	SubmissionCancelled SubmissionStatus = "cancelled"
)

// Submission is the other party's details and signature over the final
// terms, and the creator's signature over the same terms once approved.
type Submission struct {
	ID     string
	Status SubmissionStatus
	Terms  EscrowTerms
	// Message is the exact text both parties sign (Terms.Message).
	Message string
	// Wallet is the submitting wallet: the freelancer's NEAR account or the
	// approver wallet. Guest tokens are issued to it.
	Wallet                Wallet
	CounterpartySignature string
	// CounterpartyPublicKey is set when the counterparty signed with NEP-413.
	CounterpartyPublicKey string
	CreatorSignature      string
	CreatorPublicKey      string
	FailureReason         string
	CreatedAt             time.Time
}

// FreelancerSignature is a NEP-413 signature with the key that made it, as
// create_escrow takes it.
type FreelancerSignature struct {
	PublicKey string
	Signature string
}

// Signatures returns the freelancer's and the client's signature over the
// terms, given the role of the escrow's creator.
func (s Submission) Signatures(creatorRole Role) (FreelancerSignature, string) {
	if creatorRole == RoleFreelancer {
		return FreelancerSignature{PublicKey: s.CreatorPublicKey, Signature: s.CreatorSignature}, s.CounterpartySignature
	}
	return FreelancerSignature{PublicKey: s.CounterpartyPublicKey, Signature: s.CounterpartySignature}, s.CreatorSignature
}

// Milestone is a milestone's amount and, once on-chain, its state.
type Milestone struct {
	Amount        string
	Status        MilestoneStatus
	SubmittedAtMS *uint64
	RevisionCount int
}

// PendingPayout is a payout the contract holds because the transfer failed.
type PendingPayout struct {
	AccountID string
	Amount    string
}

// Escrow is an escrow as the backend tracks it: the proposal until it is
// on-chain, then a snapshot of the contract's state.
type Escrow struct {
	ID               string
	CreatorAccountID string
	CreatorRole      Role
	Status           EscrowStatus
	FeeBPS           uint16
	Milestones       []Milestone
	// The creator's own details; exactly one is set, by CreatorRole.
	Freelancer *FreelancerDetails
	Client     *ClientDetails
	// Submission is the active submission (pending, approved or created).
	Submission *Submission

	// Snapshot of get_escrow, set once the escrow is on-chain.
	ReviewWindowSec  *uint64
	Balance          string
	PendingPayouts   []PendingPayout
	ChainCreatedAtMS *uint64
	FundedAtMS       *uint64

	CreatedAt time.Time
	UpdatedAt time.Time
}

// MilestoneAmounts returns the milestone amounts in order.
func (e Escrow) MilestoneAmounts() []string {
	out := make([]string, len(e.Milestones))
	for i, m := range e.Milestones {
		out[i] = m.Amount
	}
	return out
}

// OnChain reports whether the escrow exists in the contract.
func (e Escrow) OnChain() bool { return e.ChainCreatedAtMS != nil }

// Terms returns the terms signed by both parties, once the creator approved.
func (e Escrow) Terms() (EscrowTerms, bool) {
	if e.Submission == nil || (e.Submission.Status != SubmissionApproved && e.Submission.Status != SubmissionCreated) {
		return EscrowTerms{}, false
	}
	return e.Submission.Terms, true
}

// CreatorWallet is the wallet the creator signs the terms with: their NEAR
// account as freelancer, the approver as client.
func (e Escrow) CreatorWallet() Wallet {
	if e.CreatorRole == RoleFreelancer && e.Freelancer != nil {
		return Wallet{Scheme: SchemeNEP413, Address: e.Freelancer.Account}
	}
	if e.Client != nil {
		return e.Client.Approver.Wallet()
	}
	return Wallet{}
}

// Party is a wallet with a role in an escrow, for "My escrows" (R-16).
type Party struct {
	Role   Role
	Wallet Wallet
}

// Parties returns the wallets with a role in the escrow: the freelancer
// account, the split members other than the freelancer, and the approver,
// from the creator's details and the active submission.
func (e Escrow) Parties() []Party {
	var f *FreelancerDetails
	var c *ClientDetails
	if e.Submission != nil {
		fd, cd := e.Submission.Terms.FreelancerDetails(), e.Submission.Terms.ClientDetails()
		f, c = &fd, &cd
	} else {
		f, c = e.Freelancer, e.Client
	}
	var out []Party
	add := func(p Party) {
		for _, q := range out {
			if q == p {
				return
			}
		}
		out = append(out, p)
	}
	if f != nil {
		add(Party{Role: RoleFreelancer, Wallet: Wallet{Scheme: SchemeNEP413, Address: f.Account}})
		for _, s := range f.Split {
			if s.AccountID != f.Account {
				add(Party{Role: RoleMember, Wallet: Wallet{Scheme: SchemeNEP413, Address: s.AccountID}})
			}
		}
	}
	if c != nil {
		add(Party{Role: RoleClient, Wallet: c.Approver.Wallet()})
	}
	return out
}

// ChainEscrow is the escrow as get_escrow returns it.
type ChainEscrow struct {
	EscrowID        string
	Freelancer      string
	Milestones      []Milestone
	Split           []SplitShare
	FeeBPS          uint16
	ReviewWindowSec uint64
	Approver        Approver
	RefundAccount   string
	Status          EscrowStatus
	Total           string
	Balance         string
	PendingPayouts  []PendingPayout
	CreatedAtMS     uint64
	FundedAtMS      *uint64
	// BlockHeight is the block the view was read at; older views never
	// overwrite newer ones.
	BlockHeight uint64
}

// MatchesTerms reports whether the on-chain escrow has the given terms.
func (c ChainEscrow) MatchesTerms(t EscrowTerms) bool {
	if c.EscrowID != t.EscrowID || c.Freelancer != t.Freelancer || c.FeeBPS != t.FeeBPS ||
		c.RefundAccount != t.RefundAccount || c.Approver.Label() != t.Approver.Label() ||
		len(c.Milestones) != len(t.Milestones) || len(c.Split) != len(t.Split) {
		return false
	}
	for i, m := range c.Milestones {
		if m.Amount != t.Milestones[i] {
			return false
		}
	}
	for i, s := range c.Split {
		if s != t.Split[i] {
			return false
		}
	}
	return true
}

// ContractEvent is a NEP-297 event logged by the escrow contract.
type ContractEvent struct {
	ReceiptID string
	LogIndex  int
	Event     string
	// Data is the event's JSON data.
	Data []byte
}

// Transaction is a transaction the backend sent for an escrow.
type Transaction struct {
	Hash       string
	EscrowID   string
	Method     string
	SignerID   string
	SenderID   string
	ReceiverID string
	Succeeded  bool
	// Error is the failure reported by the chain, e.g. the contract's panic
	// message.
	Error     string
	Events    []ContractEvent
	CreatedAt time.Time
}

// RevisionNote is the reason the client gave with a revision request.
type RevisionNote struct {
	Milestone int
	Reason    string
	TxHash    string
	CreatedAt time.Time
}

// MaxRevisionReasonLen bounds a revision reason, in bytes.
const MaxRevisionReasonLen = 2000

// ParseRevisionReason trims and checks a revision reason.
func ParseRevisionReason(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", InvalidArgument("reason is required")
	}
	if len(s) > MaxRevisionReasonLen {
		return "", InvalidArgument("reason must be at most %d bytes", MaxRevisionReasonLen)
	}
	return s, nil
}

// GuestToken is what a guest token grants: viewing one escrow as the wallet
// that submitted to it (R-21). It is never proof of a right in the contract.
type GuestToken struct {
	EscrowID  string
	Wallet    Wallet
	ExpiresAt time.Time
}

// FundingStatus is the funding status shown to the client (spec.md §6).
type FundingStatus string

const (
	FundingPending  FundingStatus = "PENDING"
	FundingSuccess  FundingStatus = "SUCCESS"
	FundingRefunded FundingStatus = "REFUNDED"
)

// FundingQuote is a 1Click quote for funding an escrow, and the funding
// status watched after it.
type FundingQuote struct {
	ID                string
	EscrowID          string
	OriginAsset       string
	RefundTo          string
	DepositAddress    string
	DepositMemo       string
	AmountIn          string
	AmountInFormatted string
	AmountOut         string
	Deadline          *time.Time
	Status            FundingStatus
	// OneClickStatus is the raw 1Click status, e.g. PROCESSING or FAILED;
	// empty until 1Click knows of a deposit.
	OneClickStatus string
	// Watching is true while the funding watcher still checks the status.
	Watching  bool
	CheckedAt *time.Time
	// Raw is the quote response as 1Click returned it.
	Raw       []byte
	CreatedAt time.Time
}

// Token is a token 1Click can swap from.
type Token struct {
	AssetID         string
	Symbol          string
	Blockchain      string
	Decimals        int
	Price           float64
	ContractAddress string
}

// AccessKeyStatus says how a public key relates to a NEAR account.
type AccessKeyStatus int

const (
	AccessKeyFullAccess AccessKeyStatus = iota + 1
	// AccessKeyLimited: the key is a function-call key.
	AccessKeyLimited
	// AccessKeyMissing: the account exists but the key is not on it.
	AccessKeyMissing
	AccountMissing
)

// Requester is who calls an escrow endpoint: the account of a session, the
// holder of a guest token, both or neither.
type Requester struct {
	AccountID string
	Guest     *GuestToken
}
