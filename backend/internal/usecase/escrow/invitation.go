package escrow

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/harundarat/vecrow/backend/internal/domain"
)

// InvitationInput names the other party of an escrow by exactly one of
// Username, Email and Wallet (R-15).
type InvitationInput struct {
	Username string
	Email    string
	Wallet   *WalletInput
}

// WalletInput is a wallet address with its scheme (nep413, eip191, ed25519).
type WalletInput struct {
	Scheme  string
	Address string
}

// WarningSubmitterNotInvited warns the creator and the invited account that
// the wallet that submitted is not a wallet of the invited account (R-15).
const WarningSubmitterNotInvited = "submitter_not_invited"

// Warning is shown with an escrow to one requester.
type Warning struct {
	Code    string
	Message string
}

// parseInvitation validates an invitation for an escrow created by the
// account creatorID. A username must belong to an account; an email or a
// wallet may not belong to any account yet. The creator cannot invite their
// own account.
func (s *Service) parseInvitation(ctx context.Context, creatorID string, in InvitationInput) (domain.Invitation, error) {
	username, email := strings.TrimSpace(in.Username), strings.TrimSpace(in.Email)
	n := 0
	for _, set := range []bool{username != "", email != "", in.Wallet != nil} {
		if set {
			n++
		}
	}
	if n != 1 {
		return domain.Invitation{}, domain.InvalidArgument("invitation must name exactly one of username, email or wallet")
	}

	var inv domain.Invitation
	switch {
	case username != "":
		u, err := domain.ParseUsername(username)
		if err != nil {
			return domain.Invitation{}, err
		}
		id, err := s.Invitees.AccountByUsername(ctx, u)
		if err != nil {
			return domain.Invitation{}, fmt.Errorf("escrow: find invited username: %w", err)
		}
		if id == "" {
			return domain.Invitation{}, domain.InvalidArgument("no account has the username %s", u)
		}
		inv = domain.Invitation{Kind: domain.InviteByUsername, AccountID: id, Username: u}
	case email != "":
		e, err := domain.ParseEmail(email)
		if err != nil {
			return domain.Invitation{}, err
		}
		inv = domain.Invitation{Kind: domain.InviteByEmail, Email: e}
	default:
		scheme, err := domain.ParseWalletScheme(in.Wallet.Scheme)
		if err != nil {
			return domain.Invitation{}, err
		}
		w, err := domain.NewWallet(scheme, in.Wallet.Address)
		if err != nil {
			return domain.Invitation{}, err
		}
		inv = domain.Invitation{Kind: domain.InviteByWallet, Wallet: w}
	}

	invited, err := s.Invitees.InvitedAccount(ctx, inv)
	if err != nil {
		return domain.Invitation{}, fmt.Errorf("escrow: find invited account: %w", err)
	}
	if invited == creatorID {
		return domain.Invitation{}, domain.InvalidArgument("the invitation names your own account; name the other party")
	}
	return inv, nil
}

// invitationChangeable reports whether the invitation of e can still change:
// until the escrow is on-chain, and not once it is cancelled.
func invitationChangeable(e domain.Escrow) error {
	switch e.Status {
	case domain.EscrowOpen, domain.EscrowPendingApproval, domain.EscrowApproved:
		return nil
	default:
		return domain.Conflict("escrow is %s; the invitation can only change until the escrow is on-chain", e.Status)
	}
}

// UpdateInvitation replaces the invitation of an escrow, or removes it when
// in is nil (R-15). Only the creator can, and only until the escrow is
// on-chain. The invitation grants visibility only; it never changes the
// terms.
func (s *Service) UpdateInvitation(ctx context.Context, accountID, id string, in *InvitationInput) (domain.Escrow, error) {
	e, err := s.get(ctx, id)
	if err != nil {
		return domain.Escrow{}, err
	}
	if e.CreatorAccountID != accountID {
		return domain.Escrow{}, domain.Forbidden("only the creator can change the invitation")
	}
	if err := invitationChangeable(e); err != nil {
		return domain.Escrow{}, err
	}
	var inv *domain.Invitation
	if in != nil {
		i, err := s.parseInvitation(ctx, accountID, *in)
		if err != nil {
			return domain.Escrow{}, err
		}
		inv = &i
	}
	err = s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		locked, err := s.lock(ctx, id)
		if err != nil {
			return err
		}
		if err := invitationChangeable(locked); err != nil {
			return err
		}
		if err := s.Repo.SetInvitation(ctx, id, inv); err != nil {
			return fmt.Errorf("escrow: set invitation: %w", err)
		}
		return nil
	})
	if err != nil {
		return domain.Escrow{}, err
	}
	kind := "none"
	if inv != nil {
		kind = string(inv.Kind)
	}
	s.rt.log.InfoContext(ctx, "escrow invitation changed", slog.String("escrow_id", id), slog.String("invitation", kind))
	return s.get(ctx, id)
}

// addViewer fills what the view shows to the account viewing it: whether it
// created the escrow or is invited, the invitation for the creator, and a
// warning for either of them when the submitting wallet is not a wallet of
// the invited account (R-15). Other requesters learn nothing about the
// invitation.
func (s *Service) addViewer(ctx context.Context, v *View, accountID string) error {
	e := v.Escrow
	v.Creator = e.CreatorAccountID == accountID
	if v.Creator {
		v.Invitation = e.Invitation
	}
	if e.Invitation == nil {
		return nil
	}
	invited, err := s.Invitees.InvitedAccount(ctx, *e.Invitation)
	if err != nil {
		return fmt.Errorf("escrow: find invited account: %w", err)
	}
	if invited == e.CreatorAccountID {
		// The creator linked the invited wallet or email after inviting it.
		invited = ""
	}
	v.Invited = invited != "" && invited == accountID
	if (!v.Creator && !v.Invited) || e.Submission == nil {
		return nil
	}
	ok, err := s.submitterInvited(ctx, *e.Invitation, invited, e.Submission.Wallet)
	if err != nil || ok {
		return err
	}
	v.Warnings = append(v.Warnings, submitterWarning(e, v.Creator))
	return nil
}

// submitterInvited reports whether w, the wallet that submitted, is the
// invited wallet or a wallet of the invited account.
func (s *Service) submitterInvited(ctx context.Context, inv domain.Invitation, invitedAccount string, w domain.Wallet) (bool, error) {
	if inv.Kind == domain.InviteByWallet && inv.Wallet == w {
		return true, nil
	}
	if invitedAccount == "" {
		return false, nil
	}
	ok, err := s.Wallets.IsWalletOwner(ctx, invitedAccount, w)
	if err != nil {
		return false, fmt.Errorf("escrow: check submitting wallet: %w", err)
	}
	return ok, nil
}

func submitterWarning(e domain.Escrow, creator bool) Warning {
	w := e.Submission.Wallet.Address
	pending := e.Status == domain.EscrowPendingApproval
	var msg string
	switch {
	case creator && pending:
		msg = fmt.Sprintf("The wallet that submitted, %s, is not a wallet of the party you invited. "+
			"Check the other party's details before you approve.", w)
	case creator:
		msg = fmt.Sprintf("The other party's wallet, %s, is not a wallet of the party you invited.", w)
	case pending:
		msg = fmt.Sprintf("The wallet that submitted to this escrow, %s, is not linked to your account. "+
			"If you did not submit it, tell the creator not to approve it.", w)
	default:
		msg = fmt.Sprintf("The other party's wallet in this escrow, %s, is not linked to your account.", w)
	}
	return Warning{Code: WarningSubmitterNotInvited, Message: msg}
}
