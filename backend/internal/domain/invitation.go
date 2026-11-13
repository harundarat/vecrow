package domain

// InvitationKind is how the creator of an escrow named the other party.
type InvitationKind string

const (
	InviteByUsername InvitationKind = "username"
	InviteByEmail    InvitationKind = "email"
	InviteByWallet   InvitationKind = "wallet"
)

// Invitation names the other party of an escrow so the escrow appears in
// their account (R-15). It grants visibility only: the terms never take an
// address from an invitation or from the invited account; the other party's
// addresses always come from their own signature (D-08).
type Invitation struct {
	Kind InvitationKind
	// AccountID is the invited account of an invitation by username, found
	// when the invitation is made; Username is its current username.
	AccountID string
	Username  string
	// Email is matched against verified emails only, so the invitation
	// reaches the account that verifies it, now or later.
	Email string
	// Wallet is matched against linked wallets, so the invitation reaches the
	// account that links it, now or later.
	Wallet Wallet
}

// InvitedRole is the role an invited account has in the escrow: the role
// the creator did not take.
func (e Escrow) InvitedRole() Role { return e.CreatorRole.Other() }
