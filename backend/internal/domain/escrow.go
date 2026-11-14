package domain

import (
	"encoding/hex"
	"math/big"
	"regexp"
	"strings"
)

// Limits the escrow contract enforces (architecture.md §5.1, §5.2). The
// backend checks them before anyone signs, so terms the contract would reject
// are refused early.
const (
	MaxMilestones      = 20
	MaxSplitRecipients = 10
	MaxEscrowIDLen     = 64
	TotalBPS           = 10_000
	MaxRevisions       = 2
)

// EscrowStatus is the status of an escrow. The first three exist only in the
// backend; the contract's statuses start at Created (spec.md §6, D-10).
type EscrowStatus string

const (
	EscrowOpen            EscrowStatus = "Open"
	EscrowPendingApproval EscrowStatus = "PendingApproval"
	EscrowApproved        EscrowStatus = "Approved"
	EscrowCreated         EscrowStatus = "Created"
	EscrowFunded          EscrowStatus = "Funded"
	EscrowCompleted       EscrowStatus = "Completed"
	EscrowCancelled       EscrowStatus = "Cancelled"
)

// ParseEscrowStatus validates s.
func ParseEscrowStatus(s string) (EscrowStatus, error) {
	switch st := EscrowStatus(s); st {
	case EscrowOpen, EscrowPendingApproval, EscrowApproved, EscrowCreated,
		EscrowFunded, EscrowCompleted, EscrowCancelled:
		return st, nil
	default:
		return "", InvalidArgument("unknown escrow status %q", s)
	}
}

// MilestoneStatus is the contract's milestone status.
type MilestoneStatus string

const (
	MilestonePending           MilestoneStatus = "Pending"
	MilestoneSubmitted         MilestoneStatus = "Submitted"
	MilestoneRevisionRequested MilestoneStatus = "RevisionRequested"
	MilestoneReleased          MilestoneStatus = "Released"
	MilestoneRefunded          MilestoneStatus = "Refunded"
)

// Role is a party's role in an escrow. Members (split recipients) are not a
// creator role, only a "my escrows" grouping.
type Role string

const (
	RoleFreelancer Role = "freelancer"
	RoleClient     Role = "client"
	RoleMember     Role = "member"
)

// ParseRole validates the role of an escrow creator.
func ParseRole(s string) (Role, error) {
	switch r := Role(s); r {
	case RoleFreelancer, RoleClient:
		return r, nil
	default:
		return "", InvalidArgument("role must be freelancer or client")
	}
}

// Other is the role of the other party.
func (r Role) Other() Role {
	if r == RoleFreelancer {
		return RoleClient
	}
	return RoleFreelancer
}

var escrowIDPattern = regexp.MustCompile(`^[a-z0-9_-]{1,64}$`)

// ValidateEscrowID checks the escrow ID format of the contract: 1 to 64
// characters of a-z, 0-9, '-' and '_'.
func ValidateEscrowID(id string) error {
	if !escrowIDPattern.MatchString(id) {
		return InvalidArgument("escrow ID must be 1 to %d characters of a-z, 0-9, - and _", MaxEscrowIDLen)
	}
	return nil
}

// ValidateNEARAccount checks a NEAR account ID (named, implicit or
// eth-implicit). field names it in the error.
func ValidateNEARAccount(field, account string) error {
	if !isNEARAccount(account) {
		return InvalidArgument("%s is not a valid NEAR account ID", field)
	}
	return nil
}

var maxU128 = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 128), big.NewInt(1))

var decimalPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)$`)

// ParseAmount parses a USDC amount in base units: a decimal string without
// sign or leading zeros that fits in a u128, as the contract's U128 JSON.
func ParseAmount(s string) (*big.Int, error) {
	if len(s) > 39 || !decimalPattern.MatchString(s) {
		return nil, InvalidArgument("amount %q must be a decimal integer in base units", s)
	}
	n, _ := new(big.Int).SetString(s, 10)
	if n.Cmp(maxU128) > 0 {
		return nil, InvalidArgument("amount %q is too large", s)
	}
	return n, nil
}

// ValidateMilestones checks the milestone amounts: 1 to MaxMilestones, each
// positive, with a total that fits in a u128. It returns the total.
func ValidateMilestones(milestones []string) (*big.Int, error) {
	if len(milestones) == 0 {
		return nil, InvalidArgument("at least one milestone is required")
	}
	if len(milestones) > MaxMilestones {
		return nil, InvalidArgument("at most %d milestones are allowed", MaxMilestones)
	}
	total := new(big.Int)
	for _, m := range milestones {
		n, err := ParseAmount(m)
		if err != nil {
			return nil, err
		}
		if n.Sign() == 0 {
			return nil, InvalidArgument("milestone amounts must be positive")
		}
		total.Add(total, n)
	}
	if total.Cmp(maxU128) > 0 {
		return nil, InvalidArgument("milestone total is too large")
	}
	return total, nil
}

// SplitShare is one recipient of the revenue split, in basis points.
type SplitShare struct {
	AccountID string
	BPS       uint16
}

// ValidateSplit checks the R-03 rules: 1 to MaxSplitRecipients distinct NEAR
// accounts, each with a positive share, totalling exactly TotalBPS.
func ValidateSplit(split []SplitShare) error {
	if len(split) == 0 {
		return InvalidArgument("split must have at least one recipient")
	}
	if len(split) > MaxSplitRecipients {
		return InvalidArgument("split has more than %d recipients", MaxSplitRecipients)
	}
	total := 0
	for i, s := range split {
		if err := ValidateNEARAccount("split account_id", s.AccountID); err != nil {
			return err
		}
		if s.BPS == 0 {
			return InvalidArgument("split shares must be positive")
		}
		for _, prev := range split[:i] {
			if prev.AccountID == s.AccountID {
				return InvalidArgument("split recipients must be distinct")
			}
		}
		total += int(s.BPS)
	}
	if total != TotalBPS {
		return InvalidArgument("split must total exactly %d bps", TotalBPS)
	}
	return nil
}

// Approver is the client's wallet that signs the terms and every later
// approval, revision request and cancellation (R-04, R-06).
type Approver struct {
	Scheme WalletScheme
	// AccountID and PublicKey ("ed25519:<base58>") are set for nep413.
	AccountID string
	PublicKey string
	// Address is set for eip191 (0x + 40 lowercase hex) and ed25519 (base58).
	Address string
}

// NewApprover validates an approver and normalises it to the form the
// contract writes: lowercase EVM addresses and canonical base58.
func NewApprover(scheme WalletScheme, accountID, publicKey, address string) (Approver, error) {
	switch scheme {
	case SchemeNEP413:
		if err := ValidateNEARAccount("approver account_id", accountID); err != nil {
			return Approver{}, err
		}
		key, err := ParseNEARPublicKey(publicKey)
		if err != nil {
			return Approver{}, InvalidArgument("approver public_key must be ed25519:<base58> of 32 bytes")
		}
		return Approver{Scheme: scheme, AccountID: accountID, PublicKey: FormatNEARPublicKey(key)}, nil
	case SchemeEIP191:
		if !evmAddress.MatchString(address) {
			return Approver{}, InvalidArgument("approver address must be 0x followed by 40 hex characters")
		}
		return Approver{Scheme: scheme, Address: strings.ToLower(address)}, nil
	case SchemeEd25519:
		key, ok := decodeEd25519Base58(address)
		if !ok {
			return Approver{}, InvalidArgument("approver address must be a base58 ed25519 public key")
		}
		return Approver{Scheme: scheme, Address: encodeBase58(key)}, nil
	default:
		return Approver{}, InvalidArgument("approver scheme must be one of nep413, eip191, ed25519")
	}
}

// Label is the approver as written on the "Approver:" line of the terms:
// nep413:{account_id}, eip191:0x{hex} or ed25519:{base58}.
func (a Approver) Label() string {
	if a.Scheme == SchemeNEP413 {
		return string(a.Scheme) + ":" + a.AccountID
	}
	return string(a.Scheme) + ":" + a.Address
}

// Wallet is the wallet the approver signs with, as linked to accounts.
func (a Approver) Wallet() Wallet {
	if a.Scheme == SchemeNEP413 {
		return Wallet{Scheme: a.Scheme, Address: a.AccountID}
	}
	return Wallet{Scheme: a.Scheme, Address: a.Address}
}

// ed25519Key returns the approver's ed25519 key (NEAR or Solana).
func (a Approver) ed25519Key() ([]byte, bool) {
	switch a.Scheme {
	case SchemeNEP413:
		k, err := ParseNEARPublicKey(a.PublicKey)
		return k, err == nil
	case SchemeEd25519:
		return decodeEd25519Base58(a.Address)
	default:
		return nil, false
	}
}

// nearAccountIDs are the NEAR accounts controlled by the approver's name or
// key: its account, the implicit account of its ed25519 key, or the
// eth-implicit account of its EVM address (D-18).
func (a Approver) nearAccountIDs() []string {
	var ids []string
	if a.Scheme == SchemeNEP413 {
		ids = append(ids, a.AccountID)
	}
	if a.Scheme == SchemeEIP191 {
		ids = append(ids, a.Address)
	}
	if key, ok := a.ed25519Key(); ok {
		ids = append(ids, hex.EncodeToString(key))
	}
	return ids
}

// FreelancerDetails are what the freelancer provides and signs: their NEAR
// account and the split (R-03).
type FreelancerDetails struct {
	Account string
	Split   []SplitShare
}

// Validate checks the account and the split rules.
func (d FreelancerDetails) Validate() error {
	if err := ValidateNEARAccount("freelancer", d.Account); err != nil {
		return err
	}
	return ValidateSplit(d.Split)
}

// ClientDetails are what the client provides and signs: the approver wallet
// and the NEAR account that receives refunds (R-04).
type ClientDetails struct {
	Approver      Approver
	RefundAccount string
}

// Validate checks the refund account; the approver is validated by
// NewApprover.
func (d ClientDetails) Validate() error {
	if d.Approver.Scheme == "" {
		return InvalidArgument("approver is required")
	}
	return ValidateNEARAccount("refund_account", d.RefundAccount)
}

// EscrowTerms are the terms both parties sign and create_escrow receives.
type EscrowTerms struct {
	EscrowID      string
	Freelancer    string
	Milestones    []string // USDC base units
	Split         []SplitShare
	Approver      Approver
	RefundAccount string
	FeeBPS        uint16
	DeadlineMS    uint64
}

// NewEscrowTerms combines the milestones with both parties' details.
func NewEscrowTerms(escrowID string, milestones []string, f FreelancerDetails, c ClientDetails,
	feeBPS uint16, deadlineMS uint64) EscrowTerms {
	return EscrowTerms{
		EscrowID:      escrowID,
		Freelancer:    f.Account,
		Milestones:    milestones,
		Split:         f.Split,
		Approver:      c.Approver,
		RefundAccount: c.RefundAccount,
		FeeBPS:        feeBPS,
		DeadlineMS:    deadlineMS,
	}
}

// FreelancerDetails returns the freelancer's side of the terms.
func (t EscrowTerms) FreelancerDetails() FreelancerDetails {
	return FreelancerDetails{Account: t.Freelancer, Split: t.Split}
}

// ClientDetails returns the client's side of the terms.
func (t EscrowTerms) ClientDetails() ClientDetails {
	return ClientDetails{Approver: t.Approver, RefundAccount: t.RefundAccount}
}

// Total is the sum of the milestones; the terms must be valid.
func (t EscrowTerms) Total() *big.Int {
	total := new(big.Int)
	for _, m := range t.Milestones {
		n, ok := new(big.Int).SetString(m, 10)
		if ok {
			total.Add(total, n)
		}
	}
	return total
}

// Validate applies the rules create_escrow checks before the signatures
// (R-01, R-03, R-04, D-18) for the escrow contract contractID, except the
// deadline and the fee, which depend on time and configuration, and the
// freelancer's signing key (CheckFreelancerKey).
func (t EscrowTerms) Validate(contractID string) error {
	if err := ValidateEscrowID(t.EscrowID); err != nil {
		return err
	}
	if _, err := ValidateMilestones(t.Milestones); err != nil {
		return err
	}
	if err := t.FreelancerDetails().Validate(); err != nil {
		return err
	}
	if err := t.ClientDetails().Validate(); err != nil {
		return err
	}
	if t.Freelancer == contractID {
		return InvalidArgument("the escrow contract cannot be the freelancer")
	}
	for _, s := range t.Split {
		if s.AccountID == contractID {
			return InvalidArgument("the escrow contract cannot be a split recipient")
		}
	}
	for _, id := range t.Approver.nearAccountIDs() {
		if t.isParty(id) {
			return InvalidArgument("approver must not be the freelancer or a split recipient")
		}
	}
	if t.isParty(t.RefundAccount) {
		return InvalidArgument("refund_account must not be the freelancer or a split recipient")
	}
	if t.RefundAccount == contractID {
		return InvalidArgument("refund_account must not be the escrow contract")
	}
	return nil
}

// isParty reports whether account is the freelancer or a split recipient.
func (t EscrowTerms) isParty(account string) bool {
	if account == t.Freelancer {
		return true
	}
	for _, s := range t.Split {
		if s.AccountID == account {
			return true
		}
	}
	return false
}

// CheckFreelancerKey applies the rules on the key the freelancer signs the
// terms with (D-18): an implicit freelancer account must sign with its own
// key, and the approver must not hold that key.
func (t EscrowTerms) CheckFreelancerKey(publicKey []byte) error {
	if nearImplicitAccount.MatchString(t.Freelancer) && t.Freelancer != hex.EncodeToString(publicKey) {
		return InvalidArgument("an implicit freelancer account must sign with its own key")
	}
	if key, ok := t.Approver.ed25519Key(); ok && string(key) == string(publicKey) {
		return InvalidArgument("approver must not be the freelancer or a split recipient")
	}
	return nil
}
