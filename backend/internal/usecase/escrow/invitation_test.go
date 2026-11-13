package escrow_test

import (
	"context"
	"strings"
	"testing"

	"github.com/harundarat/vecrow/backend/internal/domain"
	"github.com/harundarat/vecrow/backend/internal/usecase/escrow"
)

const accBob = "account-bob"

func (e *env) createInvited(t *testing.T, role string, inv escrow.InvitationInput) domain.Escrow {
	t.Helper()
	in := escrow.CreateInput{Role: role, Milestones: []string{"1000000"}, Invitation: &inv}
	if role == "freelancer" {
		in.AccountID, in.Details = accF, freelancerDetails()
	} else {
		in.AccountID, in.Details = accC, clientDetails()
	}
	esc, err := e.svc.Create(context.Background(), in)
	if err != nil {
		t.Fatalf("Create with %+v: %v", inv, err)
	}
	return esc
}

func TestCreateWithInvitation(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.invitees.usernames["bob"] = accBob

	esc := e.createInvited(t, "freelancer", escrow.InvitationInput{Email: " Bob@Example.COM "})
	if inv := esc.Invitation; inv == nil || inv.Kind != domain.InviteByEmail || inv.Email != "bob@example.com" {
		t.Fatalf("email invitation = %+v", esc.Invitation)
	}
	// The invitation gives no details to the terms.
	if esc.Client != nil || esc.Freelancer.Account != freelancer {
		t.Fatalf("escrow details = %+v / %+v", esc.Freelancer, esc.Client)
	}

	esc = e.createInvited(t, "freelancer", escrow.InvitationInput{Username: "Bob"})
	if inv := esc.Invitation; inv == nil || inv.Kind != domain.InviteByUsername || inv.AccountID != accBob || inv.Username != "bob" {
		t.Fatalf("username invitation = %+v", esc.Invitation)
	}

	upper := "0x" + strings.ToUpper(strings.TrimPrefix(clientEVM.Address(), "0x"))
	esc = e.createInvited(t, "client", escrow.InvitationInput{Wallet: &escrow.WalletInput{Scheme: "nep413", Address: "carol.test.near"}})
	if inv := esc.Invitation; inv == nil || inv.Kind != domain.InviteByWallet || inv.Wallet != (domain.Wallet{Scheme: domain.SchemeNEP413, Address: "carol.test.near"}) {
		t.Fatalf("wallet invitation = %+v", esc.Invitation)
	}
	esc = e.createInvited(t, "freelancer", escrow.InvitationInput{Wallet: &escrow.WalletInput{Scheme: "eip191", Address: upper}})
	if esc.Invitation.Wallet.Address != clientEVM.Address() {
		t.Fatalf("EVM address not normalised: %+v", esc.Invitation)
	}

	bad := map[string]escrow.InvitationInput{
		"nothing":            {},
		"two values":         {Username: "bob", Email: "bob@example.com"},
		"unknown username":   {Username: "nobody"},
		"invalid username":   {Username: "b"},
		"invalid email":      {Email: "bob"},
		"unknown scheme":     {Wallet: &escrow.WalletInput{Scheme: "btc", Address: "bc1q"}},
		"invalid wallet":     {Wallet: &escrow.WalletInput{Scheme: "eip191", Address: "0x1"}},
		"own wallet":         {Wallet: &escrow.WalletInput{Scheme: "nep413", Address: freelancer}},
		"own username":       {Username: "alice"},
		"own verified email": {Email: "alice@example.com"},
	}
	e.invitees.usernames["alice"] = accF
	e.invitees.emails["alice@example.com"] = accF
	for name, inv := range bad {
		inv := inv
		_, err := e.svc.Create(ctx, escrow.CreateInput{
			AccountID: accF, Role: "freelancer", Milestones: []string{"1"}, Details: freelancerDetails(), Invitation: &inv,
		})
		if !isKind(err, domain.ErrInvalidArgument) {
			t.Errorf("%s: error = %v, want invalid argument", name, err)
		}
	}
	if n := len(e.repo.escrows); n != 4 {
		t.Fatalf("stored %d escrows, want 4 (a rejected invitation stores nothing)", n)
	}
}

func TestUpdateInvitation(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	esc := e.create(t, "freelancer")

	email := &escrow.InvitationInput{Email: "bob@example.com"}
	if _, err := e.svc.UpdateInvitation(ctx, accC, esc.ID, email); !isKind(err, domain.ErrForbidden) {
		t.Fatalf("not the creator: error = %v", err)
	}
	if _, err := e.svc.UpdateInvitation(ctx, accF, "unknown", email); !isKind(err, domain.ErrNotFound) {
		t.Fatalf("unknown escrow: error = %v", err)
	}
	got, err := e.svc.UpdateInvitation(ctx, accF, esc.ID, email)
	if err != nil || got.Invitation == nil || got.Invitation.Email != "bob@example.com" {
		t.Fatalf("set email = %+v, %v", got.Invitation, err)
	}
	if _, err := e.svc.UpdateInvitation(ctx, accF, esc.ID, &escrow.InvitationInput{}); !isKind(err, domain.ErrInvalidArgument) {
		t.Fatalf("empty invitation: error = %v", err)
	}
	if e.repo.escrow(esc.ID).Invitation.Email != "bob@example.com" {
		t.Fatal("a rejected change replaced the invitation")
	}

	// It can change while the submission waits and after approval.
	e.join(t, esc)
	wallet := &escrow.InvitationInput{Wallet: &escrow.WalletInput{Scheme: "eip191", Address: clientEVM.Address()}}
	if got, err = e.svc.UpdateInvitation(ctx, accF, esc.ID, wallet); err != nil || got.Invitation.Kind != domain.InviteByWallet {
		t.Fatalf("PendingApproval: %+v, %v", got.Invitation, err)
	}
	e.approve(t, esc)
	if got, err = e.svc.UpdateInvitation(ctx, accF, esc.ID, nil); err != nil || got.Invitation != nil {
		t.Fatalf("remove while Approved: %+v, %v", got.Invitation, err)
	}
	// The terms are unchanged by any of it.
	if terms, _ := got.Terms(); terms.Approver.Wallet().Address != clientEVM.Address() {
		t.Fatalf("approver = %+v", terms.Approver)
	}

	// Not once the escrow is on-chain, nor once it is cancelled.
	if err := e.svc.CreateOnChain(ctx, esc.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.UpdateInvitation(ctx, accF, esc.ID, email); !isKind(err, domain.ErrConflict) {
		t.Fatalf("on-chain: error = %v", err)
	}
	cancelled := e.create(t, "freelancer")
	if _, err := e.svc.Cancel(ctx, cancelled.ID, escrow.CancelInput{AccountID: accF}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.UpdateInvitation(ctx, accF, cancelled.ID, email); !isKind(err, domain.ErrConflict) {
		t.Fatalf("cancelled: error = %v", err)
	}
}

func TestGetShowsInvitationAndWarnings(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	const accX = "account-x"
	esc := e.createInvited(t, "freelancer", escrow.InvitationInput{Email: "bob@example.com"})
	view := func(account string) escrow.View {
		t.Helper()
		v, err := e.svc.Get(ctx, esc.ID, domain.Requester{AccountID: account})
		if err != nil {
			t.Fatalf("Get as %q: %v", account, err)
		}
		return v
	}

	// Before anyone submits: the creator sees the invitation; nobody else
	// does, and there is nothing to warn about.
	if v := view(""); v.Creator || v.Invited || v.Invitation != nil || len(v.Warnings) != 0 {
		t.Fatalf("anonymous view = %+v", v)
	}
	if v := view(accF); !v.Creator || v.Invitation == nil || v.Invitation.Email != "bob@example.com" || len(v.Warnings) != 0 {
		t.Fatalf("creator view = %+v", v)
	}

	// The client submits with a wallet of accC. While nobody has verified
	// bob@example.com, the submitter cannot be the invited party.
	e.join(t, esc)
	v := view(accF)
	if len(v.Warnings) != 1 || v.Warnings[0].Code != escrow.WarningSubmitterNotInvited ||
		!strings.Contains(v.Warnings[0].Message, "before you approve") || !strings.Contains(v.Warnings[0].Message, clientEVM.Address()) {
		t.Fatalf("creator warnings = %+v", v.Warnings)
	}
	if v := view(accC); v.Invited || len(v.Warnings) != 0 || v.Invitation != nil {
		t.Fatalf("submitter view before verifying = %+v", v)
	}

	// accC verifies the email: the submitter is the invited party.
	e.invitees.emails["bob@example.com"] = accC
	if v := view(accF); len(v.Warnings) != 0 {
		t.Fatalf("creator warnings after the invitee verified = %+v", v.Warnings)
	}
	if v := view(accC); !v.Invited || v.Creator || len(v.Warnings) != 0 || v.Invitation != nil {
		t.Fatalf("invited submitter view = %+v", v)
	}

	// Another account holds the email: both the creator and it are warned;
	// the submitter is not.
	e.invitees.emails["bob@example.com"] = accX
	if v := view(accF); len(v.Warnings) != 1 {
		t.Fatalf("creator warnings = %+v", v.Warnings)
	}
	v = view(accX)
	if !v.Invited || len(v.Warnings) != 1 || !strings.Contains(v.Warnings[0].Message, "not linked to your account") ||
		!strings.Contains(v.Warnings[0].Message, "tell the creator not to approve") {
		t.Fatalf("invited view = %+v", v)
	}
	if v := view(accC); v.Invited || len(v.Warnings) != 0 {
		t.Fatalf("submitter view = %+v", v)
	}

	// Approving is not blocked; once on-chain the warning stays.
	e.approve(t, esc)
	if err := e.svc.CreateOnChain(ctx, esc.ID); err != nil {
		t.Fatal(err)
	}
	if v := view(accF); len(v.Warnings) != 1 || strings.Contains(v.Warnings[0].Message, "approve") {
		t.Fatalf("creator warnings on-chain = %+v", v.Warnings)
	}
	if v := view(accX); len(v.Warnings) != 1 || strings.Contains(v.Warnings[0].Message, "approve") {
		t.Fatalf("invited warnings on-chain = %+v", v.Warnings)
	}
}

func TestInvitationMatchesWalletAndAccount(t *testing.T) {
	ctx := context.Background()
	submitter := domain.Wallet{Scheme: domain.SchemeEIP191, Address: clientEVM.Address()}

	t.Run("the invited wallet itself, linked to no account", func(t *testing.T) {
		e := newEnv(t)
		delete(e.wallets.owned, accC)
		esc := e.createInvited(t, "freelancer", escrow.InvitationInput{Wallet: &escrow.WalletInput{Scheme: "eip191", Address: submitter.Address}})
		e.join(t, esc)
		if v, _ := e.svc.Get(ctx, esc.ID, domain.Requester{AccountID: accF}); len(v.Warnings) != 0 {
			t.Fatalf("warnings = %+v", v.Warnings)
		}
	})
	t.Run("another wallet of the invited account", func(t *testing.T) {
		e := newEnv(t)
		other := domain.Wallet{Scheme: domain.SchemeNEP413, Address: "bob.test.near"}
		e.wallets.owned[accC] = append(e.wallets.owned[accC], other)
		esc := e.createInvited(t, "freelancer", escrow.InvitationInput{Wallet: &escrow.WalletInput{Scheme: "nep413", Address: other.Address}})
		e.join(t, esc)
		v, _ := e.svc.Get(ctx, esc.ID, domain.Requester{AccountID: accC})
		if !v.Invited || len(v.Warnings) != 0 {
			t.Fatalf("invited view = %+v", v)
		}
		if v, _ := e.svc.Get(ctx, esc.ID, domain.Requester{AccountID: accF}); len(v.Warnings) != 0 {
			t.Fatalf("creator warnings = %+v", v.Warnings)
		}
	})
	t.Run("by username, a wallet of another account", func(t *testing.T) {
		e := newEnv(t)
		e.invitees.usernames["bob"] = accBob
		esc := e.createInvited(t, "freelancer", escrow.InvitationInput{Username: "bob"})
		e.join(t, esc)
		v, _ := e.svc.Get(ctx, esc.ID, domain.Requester{AccountID: accBob})
		if !v.Invited || len(v.Warnings) != 1 {
			t.Fatalf("invited view = %+v", v)
		}
		// A guest token holder learns nothing about the invitation.
		guest := &domain.GuestToken{EscrowID: esc.ID, Wallet: submitter}
		if v, _ := e.svc.Get(ctx, esc.ID, domain.Requester{Guest: guest}); v.Invitation != nil || len(v.Warnings) != 0 || v.Invited {
			t.Fatalf("guest view = %+v", v)
		}
	})
	t.Run("the creator linked the invited wallet later", func(t *testing.T) {
		e := newEnv(t)
		invited := domain.Wallet{Scheme: domain.SchemeNEP413, Address: "later.test.near"}
		esc := e.createInvited(t, "freelancer", escrow.InvitationInput{Wallet: &escrow.WalletInput{Scheme: "nep413", Address: invited.Address}})
		e.wallets.owned[accF] = append(e.wallets.owned[accF], invited)
		e.join(t, esc)
		v, _ := e.svc.Get(ctx, esc.ID, domain.Requester{AccountID: accF})
		if !v.Creator || v.Invited || len(v.Warnings) != 1 {
			t.Fatalf("creator view = %+v", v)
		}
	})
}
