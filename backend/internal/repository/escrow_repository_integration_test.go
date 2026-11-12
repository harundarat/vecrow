//go:build integration

package repository_test

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/harundarat/vecrow/backend/internal/domain"
	"github.com/harundarat/vecrow/backend/internal/infrastructure/database"
	"github.com/harundarat/vecrow/backend/internal/repository"
	"github.com/harundarat/vecrow/backend/internal/usecase/escrow"
)

type escrowFixture struct {
	db       *database.Postgres
	repo     *repository.EscrowRepository
	accounts *repository.AccountRepository
	sfx      string
}

func newEscrowFixture(t *testing.T) escrowFixture {
	db := openDB(t)
	return escrowFixture{
		db: db, repo: repository.NewEscrowRepository(db), accounts: repository.NewAccountRepository(db), sfx: randomSuffix(t),
	}
}

// newEscrow stores an Open escrow created by accountID as freelancer and
// deletes it after the test.
func (f escrowFixture) newEscrow(t *testing.T, accountID, freelancer string, split []domain.SplitShare) domain.Escrow {
	t.Helper()
	ctx := context.Background()
	e := domain.Escrow{
		ID:               uuid.NewString(),
		CreatorAccountID: accountID,
		CreatorRole:      domain.RoleFreelancer,
		Status:           domain.EscrowOpen,
		FeeBPS:           50,
		Milestones: []domain.Milestone{
			{Amount: "1000000", Status: domain.MilestonePending},
			{Amount: "500000", Status: domain.MilestonePending},
		},
		Freelancer: &domain.FreelancerDetails{Account: freelancer, Split: split},
	}
	if err := f.repo.Create(ctx, e); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { _, _ = f.db.Querier(ctx).Exec(ctx, `DELETE FROM escrows WHERE id = $1`, e.ID) })
	if err := f.repo.ReplaceParties(ctx, e.ID, e.Parties()); err != nil {
		t.Fatalf("ReplaceParties: %v", err)
	}
	return e
}

func (f escrowFixture) submission(t *testing.T, e domain.Escrow, approver domain.Approver, deadlineMS uint64) domain.Submission {
	t.Helper()
	terms := domain.NewEscrowTerms(e.ID, e.MilestoneAmounts(), *e.Freelancer,
		domain.ClientDetails{Approver: approver, RefundAccount: "client-" + f.sfx + ".test.near"}, e.FeeBPS, deadlineMS)
	return domain.Submission{
		ID:                    uuid.NewString(),
		Status:                domain.SubmissionPending,
		Terms:                 terms,
		Message:               terms.Message("escrow.test.near"),
		Wallet:                approver.Wallet(),
		CounterpartySignature: "0xsig",
	}
}

func TestEscrowRepository_Lifecycle(t *testing.T) {
	ctx := context.Background()
	f := newEscrowFixture(t)
	freelancerAcct := newAccount(t, f.db, f.accounts, "it_f"+f.sfx)
	clientAcct := newAccount(t, f.db, f.accounts, "it_c"+f.sfx)
	alice := "alice-" + f.sfx + ".test.near"
	bob := "bob-" + f.sfx + ".test.near"
	if _, err := f.accounts.LinkWallet(ctx, freelancerAcct, domain.Wallet{Scheme: domain.SchemeNEP413, Address: alice}); err != nil {
		t.Fatal(err)
	}
	clientWallet := evmWallet(t)
	if _, err := f.accounts.LinkWallet(ctx, clientAcct, clientWallet); err != nil {
		t.Fatal(err)
	}
	if ok, err := f.accounts.IsWalletOwner(ctx, freelancerAcct, domain.Wallet{Scheme: domain.SchemeNEP413, Address: alice}); !ok || err != nil {
		t.Fatalf("IsWalletOwner(own wallet) = %v, %v", ok, err)
	}
	if ok, err := f.accounts.IsWalletOwner(ctx, clientAcct, domain.Wallet{Scheme: domain.SchemeNEP413, Address: alice}); ok || err != nil {
		t.Fatalf("IsWalletOwner(other's wallet) = %v, %v", ok, err)
	}

	split := []domain.SplitShare{{AccountID: alice, BPS: 7000}, {AccountID: bob, BPS: 3000}}
	e := f.newEscrow(t, freelancerAcct, alice, split)

	got, err := f.repo.Get(ctx, e.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != domain.EscrowOpen || got.CreatorAccountID != freelancerAcct || got.FeeBPS != 50 ||
		!reflect.DeepEqual(got.Milestones, e.Milestones) || !reflect.DeepEqual(got.Freelancer, e.Freelancer) ||
		got.Client != nil || got.Submission != nil || got.OnChain() {
		t.Fatalf("Get = %+v", got)
	}
	if _, err := f.repo.Get(ctx, "missing-"+f.sfx); !errors.Is(err, escrow.ErrNotFound) {
		t.Fatalf("Get(missing) = %v", err)
	}

	mine, err := f.repo.ListForAccount(ctx, freelancerAcct, escrow.ListFilter{})
	if err != nil || len(mine) != 1 || mine[0].Escrow.ID != e.ID || !slices.Equal(mine[0].Roles, []domain.Role{domain.RoleFreelancer}) {
		t.Fatalf("ListForAccount(freelancer) = %+v, %v", mine, err)
	}
	if theirs, _ := f.repo.ListForAccount(ctx, clientAcct, escrow.ListFilter{}); len(theirs) != 0 {
		t.Fatalf("client sees %d escrows before joining", len(theirs))
	}

	// The client submits; a second submission in the same transaction is
	// refused without aborting it.
	approver := domain.Approver{Scheme: domain.SchemeEIP191, Address: clientWallet.Address}
	sub := f.submission(t, e, approver, uint64(time.Now().Add(time.Hour).UnixMilli()))
	err = f.db.WithinTx(ctx, func(ctx context.Context) error {
		locked, err := f.repo.Lock(ctx, e.ID)
		if err != nil || locked.ID != e.ID {
			t.Fatalf("Lock = %v", err)
		}
		if err := f.repo.InsertSubmission(ctx, e.ID, sub); err != nil {
			return err
		}
		again := sub
		again.ID = uuid.NewString()
		if err := f.repo.InsertSubmission(ctx, e.ID, again); !errors.Is(err, escrow.ErrActiveSubmission) {
			t.Fatalf("second InsertSubmission = %v", err)
		}
		if err := f.repo.SetStatus(ctx, e.ID, domain.EscrowPendingApproval); err != nil {
			return err
		}
		locked.Submission = &sub
		return f.repo.ReplaceParties(ctx, e.ID, locked.Parties())
	})
	if err != nil {
		t.Fatalf("submit transaction: %v", err)
	}
	got, _ = f.repo.Get(ctx, e.ID)
	if got.Status != domain.EscrowPendingApproval || got.Submission == nil {
		t.Fatalf("after submission: %+v", got)
	}
	gs := *got.Submission
	gs.CreatedAt = time.Time{}
	if !reflect.DeepEqual(gs, sub) {
		t.Fatalf("stored submission =\n%+v\nwant\n%+v", gs, sub)
	}
	theirs, _ := f.repo.ListForAccount(ctx, clientAcct, escrow.ListFilter{Role: domain.RoleClient})
	if len(theirs) != 1 || !slices.Equal(theirs[0].Roles, []domain.Role{domain.RoleClient}) {
		t.Fatalf("ListForAccount(client, role client) = %+v", theirs)
	}
	if l, _ := f.repo.ListForAccount(ctx, clientAcct, escrow.ListFilter{Role: domain.RoleFreelancer}); len(l) != 0 {
		t.Fatalf("role filter: %+v", l)
	}
	if l, _ := f.repo.ListForAccount(ctx, clientAcct, escrow.ListFilter{Status: domain.EscrowOpen}); len(l) != 0 {
		t.Fatalf("status filter: %+v", l)
	}

	// The creator approves.
	sub.Status, sub.CreatorSignature, sub.CreatorPublicKey = domain.SubmissionApproved, "c2ln", "ed25519:key"
	if err := f.repo.UpdateSubmission(ctx, sub); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.SetStatus(ctx, e.ID, domain.EscrowApproved); err != nil {
		t.Fatal(err)
	}
	approved, _ := f.repo.ListApproved(ctx, 1000)
	if !slices.Contains(approved, e.ID) {
		t.Fatalf("ListApproved does not have %s", e.ID)
	}
	got, _ = f.repo.Get(ctx, e.ID)
	if terms, ok := got.Terms(); !ok || terms.Approver != approver || got.Submission.CreatorSignature != "c2ln" {
		t.Fatalf("approved terms = %+v, %v", got.Submission, ok)
	}

	// Snapshots from the contract; an older one is ignored.
	submitted := uint64(time.Now().UnixMilli())
	chain := domain.ChainEscrow{
		EscrowID: e.ID, Status: domain.EscrowFunded, ReviewWindowSec: 60, Balance: "1500000", CreatedAtMS: 1, FundedAtMS: &submitted,
		Milestones: []domain.Milestone{
			{Amount: "1000000", Status: domain.MilestoneSubmitted, SubmittedAtMS: &submitted, RevisionCount: 1},
			{Amount: "500000", Status: domain.MilestonePending},
		},
		PendingPayouts: []domain.PendingPayout{{AccountID: bob, Amount: "298500"}},
		BlockHeight:    10,
	}
	if ok, err := f.repo.SaveChain(ctx, chain); !ok || err != nil {
		t.Fatalf("SaveChain = %v, %v", ok, err)
	}
	older := chain
	older.Status, older.BlockHeight = domain.EscrowCancelled, 9
	if ok, err := f.repo.SaveChain(ctx, older); ok || err != nil {
		t.Fatalf("SaveChain(older) = %v, %v", ok, err)
	}
	got, _ = f.repo.Get(ctx, e.ID)
	if got.Status != domain.EscrowFunded || !got.OnChain() || *got.ReviewWindowSec != 60 || got.Balance != "1500000" ||
		!reflect.DeepEqual(got.Milestones, chain.Milestones) || !reflect.DeepEqual(got.PendingPayouts, chain.PendingPayouts) {
		t.Fatalf("after SaveChain: %+v", got)
	}

	refs, _ := f.repo.ListDueReleases(ctx, submitted+59_999, 1000)
	if slices.Contains(refs, escrow.MilestoneRef{EscrowID: e.ID, Milestone: 0}) {
		t.Fatal("release due before the review window ended")
	}
	refs, _ = f.repo.ListDueReleases(ctx, submitted+60_000, 1000)
	if !slices.Contains(refs, escrow.MilestoneRef{EscrowID: e.ID, Milestone: 0}) || slices.Contains(refs, escrow.MilestoneRef{EscrowID: e.ID, Milestone: 1}) {
		t.Fatalf("ListDueReleases = %+v", refs)
	}
	payouts, _ := f.repo.ListPendingPayouts(ctx, 1000)
	if !slices.Contains(payouts, escrow.PayoutRef{EscrowID: e.ID, AccountID: bob}) {
		t.Fatalf("ListPendingPayouts = %+v", payouts)
	}
	if ids, _ := f.repo.ListToSync(ctx, time.Now().Add(time.Minute), 1000); !slices.Contains(ids, e.ID) {
		t.Fatal("ListToSync misses a funded escrow")
	}
	if ids, _ := f.repo.ListToSync(ctx, time.Now().Add(-time.Minute), 1000); slices.Contains(ids, e.ID) {
		t.Fatal("ListToSync lists an escrow synced just now")
	}

	// Payouts settled: no longer pending.
	chain.PendingPayouts, chain.BlockHeight = nil, 11
	if ok, err := f.repo.SaveChain(ctx, chain); !ok || err != nil {
		t.Fatal(err)
	}
	if got, _ := f.repo.Get(ctx, e.ID); len(got.PendingPayouts) != 0 {
		t.Fatalf("pending payouts = %+v", got.PendingPayouts)
	}
}

func TestEscrowRepository_TransactionsAndNotes(t *testing.T) {
	ctx := context.Background()
	f := newEscrowFixture(t)
	acct := newAccount(t, f.db, f.accounts, "it_t"+f.sfx)
	alice := "alice-" + f.sfx + ".test.near"
	e := f.newEscrow(t, acct, alice, []domain.SplitShare{{AccountID: alice, BPS: 10000}})

	first := domain.Transaction{
		Hash: "hash1-" + f.sfx, EscrowID: e.ID, Method: "create_escrow", SignerID: "relayer.test.near",
		SenderID: "relayer.test.near", ReceiverID: "escrow.test.near", Succeeded: true,
		Events: []domain.ContractEvent{
			{ReceiptID: "r2-" + f.sfx, LogIndex: 0, Event: "escrow_created", Data: []byte(`{"escrow_id": "x"}`)},
			{ReceiptID: "r1-" + f.sfx, LogIndex: 1, Event: "payout", Data: []byte(`{"paid": false}`)},
		},
	}
	second := domain.Transaction{
		Hash: "hash0-" + f.sfx, EscrowID: e.ID, Method: "request_revision", SignerID: "relayer.test.near",
		SenderID: "relayer.test.near", ReceiverID: "escrow.test.near", Succeeded: false, Error: "milestone is not Submitted",
	}
	for _, tx := range []domain.Transaction{first, first, second} {
		if err := f.repo.SaveTransaction(ctx, tx); err != nil {
			t.Fatalf("SaveTransaction(%s): %v", tx.Hash, err)
		}
	}
	txs, err := f.repo.ListTransactions(ctx, e.ID)
	if err != nil || len(txs) != 2 {
		t.Fatalf("ListTransactions = %+v, %v", txs, err)
	}
	if txs[0].Hash != first.Hash || txs[1].Hash != second.Hash || txs[1].Error != second.Error || txs[1].Succeeded {
		t.Fatalf("transactions out of order or wrong: %+v", txs)
	}
	if len(txs[0].Events) != 2 || txs[0].Events[0].Event != "escrow_created" || txs[0].Events[1].Event != "payout" ||
		string(txs[0].Events[1].Data) != `{"paid": false}` || txs[0].Events[1].LogIndex != 1 {
		t.Fatalf("events = %+v", txs[0].Events)
	}

	note := domain.RevisionNote{Milestone: 1, Reason: "fix the logo", TxHash: second.Hash}
	if err := f.repo.AddRevisionNote(ctx, e.ID, note); err != nil {
		t.Fatal(err)
	}
	notes, err := f.repo.ListRevisionNotes(ctx, e.ID)
	if err != nil || len(notes) != 1 || notes[0].Reason != note.Reason || notes[0].Milestone != 1 || notes[0].TxHash != second.Hash {
		t.Fatalf("ListRevisionNotes = %+v, %v", notes, err)
	}
}

func TestEscrowRepository_ExpiredSubmissions(t *testing.T) {
	ctx := context.Background()
	f := newEscrowFixture(t)
	acct := newAccount(t, f.db, f.accounts, "it_x"+f.sfx)
	alice := "alice-" + f.sfx + ".test.near"
	e := f.newEscrow(t, acct, alice, []domain.SplitShare{{AccountID: alice, BPS: 10000}})
	approver := domain.Approver{Scheme: domain.SchemeEIP191, Address: evmWallet(t).Address}
	deadline := uint64(time.Now().UnixMilli())
	sub := f.submission(t, e, approver, deadline)
	if err := f.repo.InsertSubmission(ctx, e.ID, sub); err != nil {
		t.Fatal(err)
	}
	if ids, _ := f.repo.ListExpiredSubmissions(ctx, deadline-1, 1000); slices.Contains(ids, e.ID) {
		t.Fatal("submission expired before its deadline")
	}
	if ids, _ := f.repo.ListExpiredSubmissions(ctx, deadline, 1000); !slices.Contains(ids, e.ID) {
		t.Fatal("submission not expired at its deadline")
	}
	sub.Status, sub.FailureReason = domain.SubmissionExpired, "deadline passed"
	if err := f.repo.UpdateSubmission(ctx, sub); err != nil {
		t.Fatal(err)
	}
	if got, _ := f.repo.Get(ctx, e.ID); got.Submission != nil {
		t.Fatal("an expired submission is still active")
	}
	// A new submission can follow.
	next := f.submission(t, e, approver, deadline+1)
	if err := f.repo.InsertSubmission(ctx, e.ID, next); err != nil {
		t.Fatalf("InsertSubmission after expiry: %v", err)
	}
}
