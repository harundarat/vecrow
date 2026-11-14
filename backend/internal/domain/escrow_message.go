package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
)

// The signed payloads below are rebuilt by the contract from its call
// arguments, so they must match architecture.md §5.3 byte for byte (D-17):
// lines separated by "\n", no trailing newline, no other whitespace.

// Message is the text both parties sign for create_escrow on contractID.
func (t EscrowTerms) Message(contractID string) string {
	split := make([]string, len(t.Split))
	for i, s := range t.Split {
		split[i] = s.AccountID + ":" + strconv.FormatUint(uint64(s.BPS), 10)
	}
	return strings.Join([]string{
		"Vecrow create_escrow",
		"Contract: " + contractID,
		"Escrow: " + t.EscrowID,
		"Freelancer: " + t.Freelancer,
		"Milestones: " + strings.Join(t.Milestones, ","),
		"Split: " + strings.Join(split, ","),
		"Approver: " + t.Approver.Label(),
		"Refund account: " + t.RefundAccount,
		"Fee: " + strconv.FormatUint(uint64(t.FeeBPS), 10) + " bps",
		"Deadline: " + strconv.FormatUint(t.DeadlineMS, 10),
	}, "\n")
}

// TermsNonce is the NEP-413 nonce for signing terms: sha256(message).
func TermsNonce(message string) [NonceSize]byte {
	return sha256.Sum256([]byte(message))
}

// ApproverAction is an action the approver signs for a milestone.
type ApproverAction string

const (
	ActionApproveMilestone ApproverAction = "approve_milestone"
	ActionRequestRevision  ApproverAction = "request_revision"
	ActionCancel           ApproverAction = "cancel"
)

// MilestoneActionMessage is the text the approver signs to approve a
// milestone or request a revision. nonce is written as 64 lowercase hex.
func MilestoneActionMessage(action ApproverAction, contractID, escrowID string, milestone uint32,
	nonce [NonceSize]byte, deadlineMS uint64) string {
	return strings.Join([]string{
		"Vecrow " + string(action),
		"Contract: " + contractID,
		"Escrow: " + escrowID,
		"Milestone: " + strconv.FormatUint(uint64(milestone), 10),
		"Nonce: " + hex.EncodeToString(nonce[:]),
		"Deadline: " + strconv.FormatUint(deadlineMS, 10),
	}, "\n")
}

// CancelMessage is the text the approver signs to cancel an unfunded escrow.
func CancelMessage(contractID, escrowID string, nonce [NonceSize]byte, deadlineMS uint64) string {
	return strings.Join([]string{
		"Vecrow cancel",
		"Contract: " + contractID,
		"Escrow: " + escrowID,
		"Nonce: " + hex.EncodeToString(nonce[:]),
		"Deadline: " + strconv.FormatUint(deadlineMS, 10),
	}, "\n")
}
