package domain

import (
	"encoding/json"
	"fmt"
)

// The JSON forms of the escrow contract's arguments and views
// (architecture.md §5.1). The backend stores terms in the same form it sends
// them to create_escrow.

// SplitShareJSON is a split share as the contract writes it.
type SplitShareJSON struct {
	AccountID string `json:"account_id"`
	BPS       uint16 `json:"bps"`
}

// ApproverJSON is an approver tagged by scheme.
type ApproverJSON struct {
	Scheme    string `json:"scheme"`
	AccountID string `json:"account_id,omitempty"`
	PublicKey string `json:"public_key,omitempty"`
	Address   string `json:"address,omitempty"`
}

// TermsJSON is the EscrowTerms argument of create_escrow.
type TermsJSON struct {
	EscrowID      string           `json:"escrow_id"`
	Freelancer    string           `json:"freelancer"`
	Milestones    []string         `json:"milestones"`
	Split         []SplitShareJSON `json:"split"`
	Approver      ApproverJSON     `json:"approver"`
	RefundAccount string           `json:"refund_account"`
	FeeBPS        uint16           `json:"fee_bps"`
	DeadlineMS    uint64           `json:"deadline_ms"`
}

// SplitJSON converts a split to its JSON form.
func SplitJSON(split []SplitShare) []SplitShareJSON {
	out := make([]SplitShareJSON, len(split))
	for i, s := range split {
		out[i] = SplitShareJSON{AccountID: s.AccountID, BPS: s.BPS}
	}
	return out
}

// ParseSplitJSON converts a split from its JSON form, without validating it.
func ParseSplitJSON(split []SplitShareJSON) []SplitShare {
	out := make([]SplitShare, len(split))
	for i, s := range split {
		out[i] = SplitShare{AccountID: s.AccountID, BPS: s.BPS}
	}
	return out
}

// JSON converts the approver to its JSON form.
func (a Approver) JSON() ApproverJSON {
	return ApproverJSON{Scheme: string(a.Scheme), AccountID: a.AccountID, PublicKey: a.PublicKey, Address: a.Address}
}

// Approver validates and normalises the approver.
func (a ApproverJSON) Approver() (Approver, error) {
	scheme, err := ParseWalletScheme(a.Scheme)
	if err != nil {
		return Approver{}, err
	}
	return NewApprover(scheme, a.AccountID, a.PublicKey, a.Address)
}

// JSON converts the terms to the form create_escrow takes.
func (t EscrowTerms) JSON() TermsJSON {
	milestones := t.Milestones
	if milestones == nil {
		milestones = []string{}
	}
	return TermsJSON{
		EscrowID:      t.EscrowID,
		Freelancer:    t.Freelancer,
		Milestones:    milestones,
		Split:         SplitJSON(t.Split),
		Approver:      t.Approver.JSON(),
		RefundAccount: t.RefundAccount,
		FeeBPS:        t.FeeBPS,
		DeadlineMS:    t.DeadlineMS,
	}
}

// Terms converts the JSON form back to terms; the approver is validated.
func (j TermsJSON) Terms() (EscrowTerms, error) {
	approver, err := j.Approver.Approver()
	if err != nil {
		return EscrowTerms{}, err
	}
	return EscrowTerms{
		EscrowID:      j.EscrowID,
		Freelancer:    j.Freelancer,
		Milestones:    j.Milestones,
		Split:         ParseSplitJSON(j.Split),
		Approver:      approver,
		RefundAccount: j.RefundAccount,
		FeeBPS:        j.FeeBPS,
		DeadlineMS:    j.DeadlineMS,
	}, nil
}

// MilestoneJSON is a milestone as get_escrow returns it.
type MilestoneJSON struct {
	Amount        string          `json:"amount"`
	Status        MilestoneStatus `json:"status"`
	SubmittedAtMS *uint64         `json:"submitted_at_ms"`
	RevisionCount int             `json:"revision_count"`
}

// PendingPayoutJSON is a pending payout as get_escrow returns it.
type PendingPayoutJSON struct {
	AccountID string `json:"account_id"`
	Amount    string `json:"amount"`
}

// EscrowViewJSON is the result of get_escrow.
type EscrowViewJSON struct {
	EscrowID        string              `json:"escrow_id"`
	Freelancer      string              `json:"freelancer"`
	Status          EscrowStatus        `json:"status"`
	Milestones      []MilestoneJSON     `json:"milestones"`
	Split           []SplitShareJSON    `json:"split"`
	FeeBPS          uint16              `json:"fee_bps"`
	ReviewWindowSec uint64              `json:"review_window_sec"`
	Approver        ApproverJSON        `json:"approver"`
	RefundAccount   string              `json:"refund_account"`
	Total           string              `json:"total"`
	Balance         string              `json:"balance"`
	PendingPayouts  []PendingPayoutJSON `json:"pending_payouts"`
	CreatedAtMS     uint64              `json:"created_at_ms"`
	FundedAtMS      *uint64             `json:"funded_at_ms"`
}

// ParseEscrowView decodes the result of get_escrow read at blockHeight. It
// returns nil for null: the contract has no such escrow.
func ParseEscrowView(b []byte, blockHeight uint64) (*ChainEscrow, error) {
	var v *EscrowViewJSON
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, fmt.Errorf("decode get_escrow: %w", err)
	}
	if v == nil {
		return nil, nil
	}
	switch v.Status {
	case EscrowCreated, EscrowFunded, EscrowCompleted, EscrowCancelled:
	default:
		return nil, fmt.Errorf("decode get_escrow: status %q", v.Status)
	}
	approver, err := v.Approver.Approver()
	if err != nil {
		return nil, fmt.Errorf("decode get_escrow: approver: %w", err)
	}
	c := &ChainEscrow{
		EscrowID:        v.EscrowID,
		Freelancer:      v.Freelancer,
		Split:           ParseSplitJSON(v.Split),
		FeeBPS:          v.FeeBPS,
		ReviewWindowSec: v.ReviewWindowSec,
		Approver:        approver,
		RefundAccount:   v.RefundAccount,
		Status:          v.Status,
		Total:           v.Total,
		Balance:         v.Balance,
		CreatedAtMS:     v.CreatedAtMS,
		FundedAtMS:      v.FundedAtMS,
		BlockHeight:     blockHeight,
	}
	for _, m := range v.Milestones {
		if !validMilestoneStatus(m.Status) {
			return nil, fmt.Errorf("decode get_escrow: milestone status %q", m.Status)
		}
		c.Milestones = append(c.Milestones, Milestone{
			Amount: m.Amount, Status: m.Status, SubmittedAtMS: m.SubmittedAtMS, RevisionCount: m.RevisionCount,
		})
	}
	for _, p := range v.PendingPayouts {
		c.PendingPayouts = append(c.PendingPayouts, PendingPayout(p))
	}
	return c, nil
}

func validMilestoneStatus(s MilestoneStatus) bool {
	switch s {
	case MilestonePending, MilestoneSubmitted, MilestoneRevisionRequested, MilestoneReleased, MilestoneRefunded:
		return true
	}
	return false
}
