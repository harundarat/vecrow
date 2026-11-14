package escrow_test

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/harundarat/vecrow/backend/internal/domain"
	"github.com/harundarat/vecrow/backend/internal/neartx"
	"github.com/harundarat/vecrow/backend/internal/usecase/escrow"
)

// fakeRepo keeps escrows in memory. Transactions are not emulated: fakeTx
// runs the function directly.
type fakeRepo struct {
	mu          sync.Mutex
	escrows     map[string]domain.Escrow
	submissions map[string][]domain.Submission // history per escrow
	parties     map[string][]domain.Party
	txs         map[string][]domain.Transaction
	notes       map[string][]domain.RevisionNote
	chainHeight map[string]uint64
	syncedAt    map[string]time.Time
	invitations map[string]domain.Invitation
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		escrows: map[string]domain.Escrow{}, submissions: map[string][]domain.Submission{},
		parties: map[string][]domain.Party{}, txs: map[string][]domain.Transaction{},
		notes: map[string][]domain.RevisionNote{}, chainHeight: map[string]uint64{}, syncedAt: map[string]time.Time{},
		invitations: map[string]domain.Invitation{},
	}
}

func (r *fakeRepo) Create(_ context.Context, e domain.Escrow) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.escrows[e.ID]; ok {
		return fmt.Errorf("duplicate escrow %s", e.ID)
	}
	e.CreatedAt = time.Now()
	r.escrows[e.ID] = e
	return nil
}

func (r *fakeRepo) Get(_ context.Context, id string) (domain.Escrow, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.escrows[id]
	if !ok {
		return domain.Escrow{}, escrow.ErrNotFound
	}
	e.Milestones = slices.Clone(e.Milestones)
	e.Submission = nil
	for _, s := range r.submissions[id] {
		switch s.Status {
		case domain.SubmissionPending, domain.SubmissionApproved, domain.SubmissionCreated:
			s := s
			e.Submission = &s
		}
	}
	e.Invitation = nil
	if inv, ok := r.invitations[id]; ok {
		e.Invitation = &inv
	}
	return e, nil
}

func (r *fakeRepo) SetInvitation(_ context.Context, id string, inv *domain.Invitation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if inv == nil {
		delete(r.invitations, id)
	} else {
		r.invitations[id] = *inv
	}
	return nil
}

func (r *fakeRepo) Lock(ctx context.Context, id string) (domain.Escrow, error) { return r.Get(ctx, id) }

func (r *fakeRepo) SetStatus(_ context.Context, id string, st domain.EscrowStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	e := r.escrows[id]
	e.Status = st
	r.escrows[id] = e
	return nil
}

func (r *fakeRepo) InsertSubmission(_ context.Context, id string, s domain.Submission) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, old := range r.submissions[id] {
		switch old.Status {
		case domain.SubmissionPending, domain.SubmissionApproved, domain.SubmissionCreated:
			return escrow.ErrActiveSubmission
		}
	}
	r.submissions[id] = append(r.submissions[id], s)
	return nil
}

func (r *fakeRepo) UpdateSubmission(_ context.Context, s domain.Submission) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, list := range r.submissions {
		for i := range list {
			if list[i].ID == s.ID {
				r.submissions[id][i] = s
				return nil
			}
		}
	}
	return fmt.Errorf("unknown submission %s", s.ID)
}

func (r *fakeRepo) ReplaceParties(_ context.Context, id string, p []domain.Party) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.parties[id] = p
	return nil
}

func (r *fakeRepo) SaveChain(_ context.Context, c domain.ChainEscrow) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c.BlockHeight < r.chainHeight[c.EscrowID] {
		return false, nil
	}
	r.chainHeight[c.EscrowID] = c.BlockHeight
	r.syncedAt[c.EscrowID] = time.Now()
	e := r.escrows[c.EscrowID]
	e.Status = c.Status
	e.Milestones = slices.Clone(c.Milestones)
	e.Balance = c.Balance
	e.PendingPayouts = slices.Clone(c.PendingPayouts)
	rw, created := c.ReviewWindowSec, c.CreatedAtMS
	e.ReviewWindowSec, e.ChainCreatedAtMS, e.FundedAtMS = &rw, &created, c.FundedAtMS
	r.escrows[c.EscrowID] = e
	return true, nil
}

func (r *fakeRepo) SaveTransaction(_ context.Context, tx domain.Transaction) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, old := range r.txs[tx.EscrowID] {
		if old.Hash == tx.Hash {
			return nil
		}
	}
	r.txs[tx.EscrowID] = append(r.txs[tx.EscrowID], tx)
	return nil
}

func (r *fakeRepo) ListTransactions(_ context.Context, id string) ([]domain.Transaction, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.txs[id]), nil
}

func (r *fakeRepo) AddRevisionNote(_ context.Context, id string, n domain.RevisionNote) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.notes[id] = append(r.notes[id], n)
	return nil
}

func (r *fakeRepo) ListRevisionNotes(_ context.Context, id string) ([]domain.RevisionNote, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.notes[id]), nil
}

func (r *fakeRepo) ListForAccount(context.Context, string, escrow.ListFilter) ([]escrow.AccountEscrow, error) {
	return nil, nil
}

func (r *fakeRepo) ListApproved(context.Context, int) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for id, e := range r.escrows {
		if e.Status == domain.EscrowApproved {
			out = append(out, id)
		}
	}
	return out, nil
}

func (r *fakeRepo) ListExpiredSubmissions(_ context.Context, nowMS uint64, _ int) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for id, list := range r.submissions {
		for _, s := range list {
			if s.Status == domain.SubmissionPending && s.Terms.DeadlineMS <= nowMS {
				out = append(out, id)
			}
		}
	}
	return out, nil
}

func (r *fakeRepo) ListDueReleases(_ context.Context, nowMS uint64, _ int) ([]escrow.MilestoneRef, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []escrow.MilestoneRef
	for id, e := range r.escrows {
		for i, m := range e.Milestones {
			if e.Status == domain.EscrowFunded && m.Status == domain.MilestoneSubmitted &&
				*m.SubmittedAtMS+*e.ReviewWindowSec*1000 <= nowMS {
				out = append(out, escrow.MilestoneRef{EscrowID: id, Milestone: uint32(i)})
			}
		}
	}
	return out, nil
}

func (r *fakeRepo) ListPendingPayouts(context.Context, int) ([]escrow.PayoutRef, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []escrow.PayoutRef
	for id, e := range r.escrows {
		for _, p := range e.PendingPayouts {
			out = append(out, escrow.PayoutRef{EscrowID: id, AccountID: p.AccountID})
		}
	}
	return out, nil
}

func (r *fakeRepo) ListToSync(context.Context, time.Time, int) ([]string, error) { return nil, nil }

func (r *fakeRepo) escrow(id string) domain.Escrow {
	e, _ := r.Get(context.Background(), id)
	return e
}

type fakeTx struct{}

func (fakeTx) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error { return fn(ctx) }

type fakeWallets struct{ owned map[string][]domain.Wallet }

func (f fakeWallets) IsWalletOwner(_ context.Context, account string, w domain.Wallet) (bool, error) {
	return slices.Contains(f.owned[account], w), nil
}

// fakeInvitees resolves usernames and verified emails from maps, and wallets
// from fakeWallets.
type fakeInvitees struct {
	usernames map[string]string // username -> account
	emails    map[string]string // verified email -> account
	wallets   fakeWallets
}

func (f fakeInvitees) AccountByUsername(_ context.Context, username string) (string, error) {
	return f.usernames[username], nil
}

func (f fakeInvitees) InvitedAccount(_ context.Context, inv domain.Invitation) (string, error) {
	switch inv.Kind {
	case domain.InviteByUsername:
		return inv.AccountID, nil
	case domain.InviteByEmail:
		return f.emails[inv.Email], nil
	case domain.InviteByWallet:
		for account, ws := range f.wallets.owned {
			if slices.Contains(ws, inv.Wallet) {
				return account, nil
			}
		}
		return "", nil
	}
	return "", fmt.Errorf("unknown invitation kind %q", inv.Kind)
}

// fakeKeys reports full access for registered (account, key) pairs and a
// missing account otherwise.
type fakeKeys struct{ full map[string]bool }

func (f fakeKeys) Status(_ context.Context, account, pk string) (domain.AccessKeyStatus, error) {
	if f.full[account+"|"+pk] {
		return domain.AccessKeyFullAccess, nil
	}
	return domain.AccountMissing, nil
}

type fakeGuests struct{ issued []domain.GuestToken }

func (f *fakeGuests) Issue(_ context.Context, id string, w domain.Wallet) (string, time.Time, error) {
	exp := time.Now().Add(time.Hour)
	f.issued = append(f.issued, domain.GuestToken{EscrowID: id, Wallet: w, ExpiresAt: exp})
	return "guest-token", exp, nil
}

// fakeContract emulates the contract enough for the use cases: it stores
// escrows created with create_escrow and records every call.
type fakeContract struct {
	mu      sync.Mutex
	id      string
	escrows map[string]*domain.ChainEscrow
	calls   []string
	// reject makes the next call fail in the contract with this message.
	reject string
	// unavailable makes calls fail without an outcome.
	unavailable bool
	height      uint64
	relayed     []neartx.SignedDelegateAction
	lastAction  escrow.ApproverAction
}

func newFakeContract(id string) *fakeContract {
	return &fakeContract{id: id, escrows: map[string]*domain.ChainEscrow{}}
}

func (c *fakeContract) ID() string { return c.id }

func (c *fakeContract) GetEscrow(_ context.Context, id string) (*domain.ChainEscrow, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.unavailable {
		return nil, fmt.Errorf("rpc down")
	}
	e, ok := c.escrows[id]
	if !ok {
		return nil, nil
	}
	c.height++
	cp := *e
	cp.Milestones = slices.Clone(e.Milestones)
	cp.PendingPayouts = slices.Clone(e.PendingPayouts)
	cp.BlockHeight = c.height
	return &cp, nil
}

func (c *fakeContract) tx(method string, events ...domain.ContractEvent) (domain.Transaction, error) {
	c.calls = append(c.calls, method)
	if c.unavailable {
		return domain.Transaction{}, fmt.Errorf("outcome unknown")
	}
	tx := domain.Transaction{Hash: fmt.Sprintf("tx%d", len(c.calls)), Method: method, SignerID: "relayer.test.near",
		SenderID: "relayer.test.near", ReceiverID: c.id, Succeeded: true, Events: events}
	if c.reject != "" {
		tx.Succeeded, tx.Error, tx.Events = false, c.reject, nil
		c.reject = ""
	}
	return tx, nil
}

func event(name string, data any) domain.ContractEvent {
	b, _ := json.Marshal(data)
	return domain.ContractEvent{ReceiptID: "r", Event: name, Data: b}
}

func (c *fakeContract) CreateEscrow(_ context.Context, t domain.EscrowTerms, f domain.FreelancerSignature, client string) (domain.Transaction, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.escrows[t.EscrowID]; ok && c.reject == "" {
		c.reject = "escrow already exists"
	}
	tx, err := c.tx("create_escrow", event("escrow_created", map[string]string{"escrow_id": t.EscrowID}))
	if err != nil || !tx.Succeeded {
		return tx, err
	}
	if f.Signature == "" || f.PublicKey == "" || client == "" {
		panic("create_escrow without both signatures")
	}
	ms := make([]domain.Milestone, len(t.Milestones))
	for i, a := range t.Milestones {
		ms[i] = domain.Milestone{Amount: a, Status: domain.MilestonePending}
	}
	c.escrows[t.EscrowID] = &domain.ChainEscrow{
		EscrowID: t.EscrowID, Freelancer: t.Freelancer, Milestones: ms, Split: t.Split, FeeBPS: t.FeeBPS,
		ReviewWindowSec: 60, Approver: t.Approver, RefundAccount: t.RefundAccount, Status: domain.EscrowCreated,
		Balance: "0", CreatedAtMS: 1,
	}
	return tx, nil
}

func (c *fakeContract) setMilestone(id string, i int, m domain.Milestone) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.escrows[id].Milestones[i] = m
}

func (c *fakeContract) ApproveMilestone(_ context.Context, id string, m uint32, a escrow.ApproverAction) (domain.Transaction, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastAction = a
	tx, err := c.tx("approve_milestone")
	if err == nil && tx.Succeeded {
		c.escrows[id].Milestones[m].Status = domain.MilestoneReleased
	}
	return tx, err
}

func (c *fakeContract) RequestRevision(_ context.Context, id string, m uint32, a escrow.ApproverAction) (domain.Transaction, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastAction = a
	tx, err := c.tx("request_revision")
	if err == nil && tx.Succeeded {
		c.escrows[id].Milestones[m].Status = domain.MilestoneRevisionRequested
		c.escrows[id].Milestones[m].RevisionCount++
	}
	return tx, err
}

func (c *fakeContract) Cancel(_ context.Context, id string, a escrow.ApproverAction) (domain.Transaction, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastAction = a
	tx, err := c.tx("cancel")
	if err == nil && tx.Succeeded {
		c.escrows[id].Status = domain.EscrowCancelled
	}
	return tx, err
}

func (c *fakeContract) AutoRelease(_ context.Context, id string, m uint32) (domain.Transaction, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	tx, err := c.tx("auto_release")
	if err == nil && tx.Succeeded {
		c.escrows[id].Milestones[m].Status = domain.MilestoneReleased
	}
	return tx, err
}

func (c *fakeContract) RetryPayout(_ context.Context, id, account string) (domain.Transaction, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	tx, err := c.tx("retry_payout")
	if err == nil && tx.Succeeded {
		e := c.escrows[id]
		e.PendingPayouts = slices.DeleteFunc(e.PendingPayouts, func(p domain.PendingPayout) bool { return p.AccountID == account })
	}
	return tx, err
}

func (c *fakeContract) Relay(_ context.Context, id string, a neartx.SignedDelegateAction) (domain.Transaction, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.relayed = append(c.relayed, a)
	fc := a.DelegateAction.Actions[0].(neartx.FunctionCall)
	tx, err := c.tx(fc.MethodName)
	if err != nil || !tx.Succeeded {
		return tx, err
	}
	var args struct {
		Milestone *uint32 `json:"milestone"`
	}
	_ = json.Unmarshal(fc.Args, &args)
	e := c.escrows[id]
	switch fc.MethodName {
	case "submit_milestone":
		now := uint64(1000)
		e.Milestones[*args.Milestone].Status = domain.MilestoneSubmitted
		e.Milestones[*args.Milestone].SubmittedAtMS = &now
	case "cancel":
		e.Status = domain.EscrowCancelled
	case "refund_unsubmitted":
		for i := range e.Milestones {
			if e.Milestones[i].Status == domain.MilestonePending {
				e.Milestones[i].Status = domain.MilestoneRefunded
			}
		}
	}
	tx.SenderID = a.DelegateAction.SenderID
	return tx, nil
}

type fakeStorage struct {
	mu         sync.Mutex
	registered map[string]bool
	registers  []string
}

func (f *fakeStorage) IsRegistered(_ context.Context, a string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.registered[a], nil
}

func (f *fakeStorage) Register(_ context.Context, _ string, a string) (domain.Transaction, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.registered[a] = true
	f.registers = append(f.registers, a)
	return domain.Transaction{Hash: "storage-" + a, Method: "storage_deposit", Succeeded: true}, nil
}
