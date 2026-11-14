package escrow_test

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/harundarat/vecrow/backend/internal/domain"
	"github.com/harundarat/vecrow/backend/internal/usecase/escrow"
)

const treasury = "treasury.test.near"

// addTx stores a transaction with events for the escrow, as the contract
// adapter would after sending it.
func (e *env) addTx(t *testing.T, escrowID, hash, method string, at time.Time, events ...domain.ContractEvent) {
	t.Helper()
	tx := domain.Transaction{Hash: hash, EscrowID: escrowID, Method: method, Succeeded: true, Events: events, CreatedAt: at}
	if err := e.repo.SaveTransaction(context.Background(), tx); err != nil {
		t.Fatal(err)
	}
}

// setPending replaces the stored snapshot of the escrow's pending payouts.
func (e *env) setPending(id string, at time.Time, p ...domain.PendingPayout) {
	e.repo.mu.Lock()
	defer e.repo.mu.Unlock()
	esc := e.repo.escrows[id]
	esc.PendingPayouts, esc.UpdatedAt = p, at
	e.repo.escrows[id] = esc
}

func released(id string, m uint32, amount, fee string) domain.ContractEvent {
	return event("milestone_released", map[string]any{"escrow_id": id, "milestone": m, "amount": amount, "fee": fee, "by": "approve_milestone"})
}

func payout(id, account, amount string, paid bool) domain.ContractEvent {
	return event("payout", map[string]any{"escrow_id": id, "account_id": account, "amount": amount, "paid": paid})
}

func refunded(id string, amount string, milestones ...uint32) domain.ContractEvent {
	return event("milestones_refunded", map[string]any{"escrow_id": id, "milestones": milestones, "amount": amount, "refund_account": refundAcct})
}

func rowEqual(a, b row) bool {
	return slices.Equal(a.milestones, b.milestones) && a.amount == b.amount && a.recipient == b.recipient &&
		a.tx == b.tx && a.status == b.status && a.at.Equal(b.at)
}

type row struct {
	milestones []uint32
	amount     string
	recipient  string
	tx         string
	status     string
	at         time.Time
}

func rows(payouts []escrow.Payout) []row {
	out := make([]row, len(payouts))
	for i, p := range payouts {
		out[i] = row{p.Milestones, p.Amount.String(), p.Recipient, p.TxHash, p.Status, p.Time}
	}
	return out
}

func TestPayoutsOnlyForParties(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	esc := e.onChain(t, "freelancer")
	e.wallets.owned["account-member"] = []domain.Wallet{{Scheme: domain.SchemeNEP413, Address: member}}
	e.wallets.owned["account-stranger"] = []domain.Wallet{{Scheme: domain.SchemeNEP413, Address: "stranger.test.near"}}
	approver := domain.Wallet{Scheme: domain.SchemeEIP191, Address: clientEVM.Address()}

	allowed := map[string]domain.Requester{
		"creator":                {AccountID: accF},
		"approver's account":     {AccountID: accC},
		"split member's account": {AccountID: "account-member"},
		"approver's guest token": {Guest: &domain.GuestToken{EscrowID: esc.ID, Wallet: approver}},
		"stranger with a party's guest token": {
			AccountID: "account-stranger", Guest: &domain.GuestToken{EscrowID: esc.ID, Wallet: approver},
		},
	}
	for name, req := range allowed {
		if _, err := e.svc.Payouts(ctx, esc.ID, req); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}

	denied := map[string]struct {
		req  domain.Requester
		kind error
	}{
		"nobody":           {domain.Requester{}, domain.ErrUnauthorized},
		"stranger account": {domain.Requester{AccountID: "account-stranger"}, domain.ErrForbidden},
		"guest token for another escrow": {
			domain.Requester{AccountID: accF, Guest: &domain.GuestToken{EscrowID: "another", Wallet: approver}}, domain.ErrUnauthorized,
		},
		"guest token of a wallet that is not a party": {
			domain.Requester{Guest: &domain.GuestToken{EscrowID: esc.ID, Wallet: domain.Wallet{Scheme: domain.SchemeNEP413, Address: "stranger.test.near"}}},
			domain.ErrForbidden,
		},
	}
	for name, c := range denied {
		if _, err := e.svc.Payouts(ctx, esc.ID, c.req); !isKind(err, c.kind) {
			t.Errorf("%s: %v, want %v", name, err, c.kind)
		}
	}
	if _, err := e.svc.Payouts(ctx, "nope", domain.Requester{AccountID: accF}); !isKind(err, domain.ErrNotFound) {
		t.Errorf("unknown escrow: %v", err)
	}
}

func TestPayoutsForInvitedAccountOnlyOnceAParty(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.invitees.usernames["client"] = accC
	esc := e.createInvited(t, "freelancer", escrow.InvitationInput{Username: "client"})
	// An invitation grants visibility, not the export: the invited account
	// is no party until one of its wallets is.
	if _, err := e.svc.Payouts(ctx, esc.ID, domain.Requester{AccountID: accC}); !isKind(err, domain.ErrForbidden) {
		t.Fatalf("invited account before joining: %v", err)
	}
	e.join(t, esc)
	if _, err := e.svc.Payouts(ctx, esc.ID, domain.Requester{AccountID: accC}); err != nil {
		t.Fatalf("invited account after joining with its wallet: %v", err)
	}
}

func TestPayoutsRowsFromStoredEvents(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	esc := e.onChain(t, "freelancer")
	id := esc.ID
	t0 := start.Add(time.Hour)
	t1, t2, t3, t4 := t0.Add(time.Minute), t0.Add(2*time.Minute), t0.Add(3*time.Minute), t0.Add(4*time.Minute)

	// Milestone 0: the fee to the treasury, 70/30 split; the member's
	// transfer fails and is held as pending.
	e.addTx(t, id, "tx-approve", "approve_milestone", t0,
		released(id, 0, "1000000", "5000"),
		payout(id, treasury, "5000", true),
		payout(id, freelancer, "696500", true),
		payout(id, member, "298500", false),
		// An event of another escrow in the same transaction is not ours.
		payout("other", freelancer, "1", true),
	)
	// Milestones 1 and 2 refunded in one transfer.
	e.addTx(t, id, "tx-refund", "refund_unsubmitted", t1,
		refunded(id, "700000", 1, 2),
		payout(id, refundAcct, "700000", true),
	)
	// A failed retry leaves the member's payout pending.
	e.addTx(t, id, "tx-retry-1", "retry_payout", t2, payout(id, member, "298500", false))
	// A transaction without events, e.g. a storage registration.
	e.addTx(t, id, "tx-storage", "storage_deposit", t2)
	e.setPending(id, t2, domain.PendingPayout{AccountID: member, Amount: "298500"})

	got, err := e.svc.Payouts(ctx, id, domain.Requester{AccountID: accF})
	if err != nil {
		t.Fatal(err)
	}
	want := []row{
		{[]uint32{0}, "5000", treasury, "tx-approve", escrow.PayoutPaid, t0},
		{[]uint32{0}, "696500", freelancer, "tx-approve", escrow.PayoutPaid, t0},
		{[]uint32{0}, "298500", member, "tx-approve", escrow.PayoutPending, t0},
		{[]uint32{1, 2}, "700000", refundAcct, "tx-refund", escrow.PayoutPaid, t1},
	}
	if g := rows(got); !slices.EqualFunc(g, want, rowEqual) {
		t.Fatalf("rows before the retry:\n got %+v\nwant %+v", g, want)
	}
	for _, p := range got {
		if p.EscrowID != id {
			t.Fatalf("escrow of %+v", p)
		}
	}

	// The retry that succeeds pays the pending row: it is paid, with the
	// retry's time and hash, in the retry's place.
	e.addTx(t, id, "tx-retry-2", "retry_payout", t3, payout(id, member, "298500", true))
	e.setPending(id, t3)
	got, err = e.svc.Payouts(ctx, id, domain.Requester{AccountID: accF})
	if err != nil {
		t.Fatal(err)
	}
	want = []row{
		{[]uint32{0}, "5000", treasury, "tx-approve", escrow.PayoutPaid, t0},
		{[]uint32{0}, "696500", freelancer, "tx-approve", escrow.PayoutPaid, t0},
		{[]uint32{1, 2}, "700000", refundAcct, "tx-refund", escrow.PayoutPaid, t1},
		{[]uint32{0}, "298500", member, "tx-retry-2", escrow.PayoutPaid, t3},
	}
	if g := rows(got); !slices.EqualFunc(g, want, rowEqual) {
		t.Fatalf("rows after the retry:\n got %+v\nwant %+v", g, want)
	}

	// Pending amounts the index cannot explain still appear: the snapshot
	// holds a fee the treasury could not receive, from a release the
	// backend did not send.
	e.setPending(id, t4, domain.PendingPayout{AccountID: treasury, Amount: "2500"})
	got, _ = e.svc.Payouts(ctx, id, domain.Requester{AccountID: accF})
	last := rows(got)[len(got)-1]
	if !rowEqual(last, row{nil, "2500", treasury, "", escrow.PayoutPending, t4}) {
		t.Fatalf("unexplained pending row = %+v", last)
	}
}

func TestPayoutsRetryPaysEveryPendingRowOfTheRecipient(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	id := e.onChain(t, "freelancer").ID
	t0 := start.Add(time.Hour)
	t1, t2 := t0.Add(time.Minute), t0.Add(2*time.Minute)

	e.addTx(t, id, "tx-m0", "approve_milestone", t0, released(id, 0, "1000000", "0"), payout(id, member, "300000", false))
	e.addTx(t, id, "tx-m1", "auto_release", t1, released(id, 1, "500000", "0"), payout(id, member, "150000", false))
	// The contract adds both up and pays them in one transfer; the extra
	// 50 comes from a failed payout the index does not have.
	e.addTx(t, id, "tx-retry", "retry_payout", t2, payout(id, member, "450050", true))
	// Later the member is owed again, from a payout the index does not have.
	e.setPending(id, t2, domain.PendingPayout{AccountID: member, Amount: "450000"})

	got, err := e.svc.Payouts(ctx, id, domain.Requester{AccountID: accF})
	if err != nil {
		t.Fatal(err)
	}
	want := []row{
		{[]uint32{0}, "300000", member, "tx-retry", escrow.PayoutPaid, t2},
		{[]uint32{1}, "150000", member, "tx-retry", escrow.PayoutPaid, t2},
		{nil, "50", member, "tx-retry", escrow.PayoutPaid, t2},
		{nil, "450000", member, "", escrow.PayoutPending, t2},
	}
	if g := rows(got); !slices.EqualFunc(g, want, rowEqual) {
		t.Fatalf("rows:\n got %+v\nwant %+v", g, want)
	}
}

func TestPayoutsKeepPendingRowsWithoutAPayingTransaction(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	id := e.onChain(t, "freelancer").ID
	t0 := start.Add(time.Hour)

	e.addTx(t, id, "tx-m0", "approve_milestone", t0, released(id, 0, "1000000", "0"), payout(id, member, "300000", false))
	// A snapshot without the pending payout (stale, or paid by a retry the
	// backend did not send) does not turn the row paid.
	e.setPending(id, t0)
	got, err := e.svc.Payouts(ctx, id, domain.Requester{AccountID: accF})
	if err != nil {
		t.Fatal(err)
	}
	want := []row{{[]uint32{0}, "300000", member, "tx-m0", escrow.PayoutPending, t0}}
	if g := rows(got); !slices.EqualFunc(g, want, rowEqual) {
		t.Fatalf("rows:\n got %+v\nwant %+v", g, want)
	}
}

func TestPayoutsRejectMalformedEvents(t *testing.T) {
	ctx := context.Background()
	for name, ev := range map[string]domain.ContractEvent{
		"amount not a number": event("payout", map[string]any{"escrow_id": "%s", "account_id": member, "amount": "12a", "paid": true}),
		"negative amount":     event("payout", map[string]any{"escrow_id": "%s", "account_id": member, "amount": "-1", "paid": true}),
		"not JSON":            {ReceiptID: "r", Event: "payout", Data: []byte("{")},
	} {
		e := newEnv(t)
		id := e.onChain(t, "freelancer").ID
		ev.Data = []byte(strings.ReplaceAll(string(ev.Data), "%s", id))
		e.addTx(t, id, "tx-bad", "approve_milestone", start, released(id, 0, "1", "0"), ev)
		if _, err := e.svc.Payouts(ctx, id, domain.Requester{AccountID: accF}); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	e := newEnv(t)
	id := e.onChain(t, "freelancer").ID
	e.setPending(id, start, domain.PendingPayout{AccountID: member, Amount: "1.5"})
	if _, err := e.svc.Payouts(ctx, id, domain.Requester{AccountID: accF}); err == nil {
		t.Error("malformed pending amount: no error")
	}
}
