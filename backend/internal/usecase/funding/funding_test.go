package funding_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/harundarat/vecrow/backend/internal/domain"
	"github.com/harundarat/vecrow/backend/internal/usecase/funding"
)

const (
	contractID  = "escrow.test.near"
	escrowID    = "6f1c2a5e-8d3b-4c1e-9f00-1234567890ab"
	approverEVM = "0x7e5f4552091a69125d5dfcb7b8c2659029395bdf"
)

var (
	start            = time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	approverWallet   = domain.Wallet{Scheme: domain.SchemeEIP191, Address: approverEVM}
	freelancerWallet = domain.Wallet{Scheme: domain.SchemeNEP413, Address: "alice.test.near"}
)

type fakeEscrows struct {
	escrows map[string]domain.Escrow
	// chain is the status the contract reports; Sync copies it.
	chain   map[string]domain.EscrowStatus
	syncs   int
	syncErr error
}

func (f *fakeEscrows) ContractID() string { return contractID }

func (f *fakeEscrows) Escrow(_ context.Context, id string) (domain.Escrow, error) {
	e, ok := f.escrows[id]
	if !ok {
		return domain.Escrow{}, domain.NotFound("escrow not found")
	}
	return e, nil
}

func (f *fakeEscrows) Sync(_ context.Context, id string) error {
	f.syncs++
	if f.syncErr != nil {
		return f.syncErr
	}
	if st, ok := f.chain[id]; ok {
		e := f.escrows[id]
		e.Status = st
		f.escrows[id] = e
	}
	return nil
}

type fakeWallets map[string][]domain.Wallet

func (f fakeWallets) IsWalletOwner(_ context.Context, account string, w domain.Wallet) (bool, error) {
	return slices.Contains(f[account], w), nil
}

type fakeRepo struct{ quotes []domain.FundingQuote }

func (r *fakeRepo) InsertQuote(_ context.Context, q domain.FundingQuote) error {
	r.quotes = append(r.quotes, q)
	return nil
}

func (r *fakeRepo) GetQuote(_ context.Context, id string) (domain.FundingQuote, error) {
	for _, q := range r.quotes {
		if q.ID == id {
			return q, nil
		}
	}
	return domain.FundingQuote{}, funding.ErrNotFound
}

func (r *fakeRepo) LatestQuote(_ context.Context, escrowID string) (domain.FundingQuote, error) {
	for i := len(r.quotes) - 1; i >= 0; i-- {
		if r.quotes[i].EscrowID == escrowID {
			return r.quotes[i], nil
		}
	}
	return domain.FundingQuote{}, funding.ErrNotFound
}

func (r *fakeRepo) UpdateStatus(_ context.Context, id string, st domain.FundingStatus, raw string, watching bool) error {
	for i := range r.quotes {
		if r.quotes[i].ID == id {
			now := start
			r.quotes[i].Status, r.quotes[i].OneClickStatus, r.quotes[i].Watching, r.quotes[i].CheckedAt = st, raw, watching, &now
		}
	}
	return nil
}

func (r *fakeRepo) ListWatching(_ context.Context, before time.Time, _ int) ([]string, error) {
	var out []string
	for _, q := range r.quotes {
		if q.Watching && (q.CheckedAt == nil || q.CheckedAt.Before(before)) {
			out = append(out, q.ID)
		}
	}
	return out, nil
}

type fakeOneClick struct {
	requests   []funding.QuoteRequest
	quote      funding.Quote
	quoteErr   error
	status     string
	statusErr  error
	tokens     []domain.Token
	tokenErr   error
	tokenCalls int
}

func (f *fakeOneClick) Tokens(context.Context) ([]domain.Token, error) {
	f.tokenCalls++
	return f.tokens, f.tokenErr
}

func (f *fakeOneClick) Quote(_ context.Context, r funding.QuoteRequest) (funding.Quote, error) {
	f.requests = append(f.requests, r)
	if f.quoteErr != nil {
		return funding.Quote{}, f.quoteErr
	}
	q := f.quote
	if q.AmountOut == "" {
		q.AmountOut = r.Amount
	}
	return q, nil
}

func (f *fakeOneClick) Status(context.Context, string, string) (string, error) {
	return f.status, f.statusErr
}

type fixture struct {
	svc      *funding.Service
	escrows  *fakeEscrows
	repo     *fakeRepo
	oneclick *fakeOneClick
	now      time.Time
}

func newFixture(t *testing.T, status domain.EscrowStatus) *fixture {
	t.Helper()
	approver, err := domain.NewApprover(domain.SchemeEIP191, "", "", approverEVM)
	if err != nil {
		t.Fatal(err)
	}
	terms := domain.EscrowTerms{
		EscrowID: escrowID, Freelancer: "alice.test.near", Milestones: []string{"1000000", "500000"},
		Split: []domain.SplitShare{{AccountID: "alice.test.near", BPS: 10000}}, Approver: approver,
		RefundAccount: "client.test.near", FeeBPS: 50, DeadlineMS: 1,
	}
	e := domain.Escrow{ID: escrowID, Status: status, CreatorRole: domain.RoleFreelancer,
		Submission: &domain.Submission{Status: domain.SubmissionCreated, Terms: terms, Wallet: approverWallet}}
	f := &fixture{
		escrows:  &fakeEscrows{escrows: map[string]domain.Escrow{escrowID: e}, chain: map[string]domain.EscrowStatus{}},
		repo:     &fakeRepo{},
		oneclick: &fakeOneClick{quote: funding.Quote{DepositAddress: "0xdeposit", AmountIn: "1000", AmountInFormatted: "0.001", Raw: []byte(`{}`)}},
		now:      start,
	}
	wallets := fakeWallets{"acct-client": {approverWallet}, "acct-freelancer": {freelancerWallet}}
	f.svc = funding.NewService(f.escrows, wallets, f.repo, f.oneclick, funding.Config{
		USDCAssetID: "nep141:usdc.test.near", SlippageBPS: 100, QuoteTTL: time.Hour, WatchGrace: 24 * time.Hour,
		RefreshAfter: 5 * time.Second, TokensTTL: 5 * time.Minute,
	}, funding.WithClock(func() time.Time { return f.now }))
	return f
}

var (
	approverGuest = domain.Requester{Guest: &domain.GuestToken{EscrowID: escrowID, Wallet: approverWallet}}
	quoteInput    = funding.QuoteInput{OriginAsset: "nep141:eth.omft.near", RefundTo: approverEVM}
)

func TestQuoteRequiresTheApprover(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t, domain.EscrowCreated)
	tests := map[string]struct {
		req  domain.Requester
		kind error
	}{
		"nothing":                {domain.Requester{}, domain.ErrUnauthorized},
		"token for other escrow": {domain.Requester{Guest: &domain.GuestToken{EscrowID: "other", Wallet: approverWallet}}, domain.ErrUnauthorized},
		"freelancer's token":     {domain.Requester{Guest: &domain.GuestToken{EscrowID: escrowID, Wallet: freelancerWallet}}, domain.ErrForbidden},
		"freelancer's session":   {domain.Requester{AccountID: "acct-freelancer"}, domain.ErrForbidden},
	}
	for name, tt := range tests {
		if _, err := f.svc.RequestQuote(ctx, escrowID, tt.req, quoteInput); !errors.Is(err, tt.kind) {
			t.Errorf("%s: %v, want %v", name, err, tt.kind)
		}
	}
	if len(f.oneclick.requests) != 0 {
		t.Fatal("1Click was asked for a quote without the approver")
	}
	for name, req := range map[string]domain.Requester{"guest token": approverGuest, "session": {AccountID: "acct-client"}} {
		if _, err := f.svc.RequestQuote(ctx, escrowID, req, quoteInput); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestQuoteRequest(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t, domain.EscrowCreated)
	q, err := f.svc.RequestQuote(ctx, escrowID, approverGuest, quoteInput)
	if err != nil {
		t.Fatal(err)
	}
	want := funding.QuoteRequest{
		OriginAsset: "nep141:eth.omft.near", DestinationAsset: "nep141:usdc.test.near", Amount: "1500000",
		RefundTo: approverEVM, Recipient: contractID, CustomRecipientMsg: escrowID, SlippageBPS: 100,
		Deadline: start.Add(time.Hour),
	}
	if got := f.oneclick.requests[0]; got != want {
		t.Fatalf("quote request = %+v\nwant %+v", got, want)
	}
	if q.DepositAddress != "0xdeposit" || q.Status != domain.FundingPending || !q.Watching || q.AmountOut != "1500000" ||
		len(f.repo.quotes) != 1 || !q.Deadline.Equal(start.Add(time.Hour)) {
		t.Fatalf("quote = %+v", q)
	}

	f.oneclick.quoteErr = &funding.QuoteRejectedError{Message: "Amount is too low"}
	if _, err := f.svc.RequestQuote(ctx, escrowID, approverGuest, quoteInput); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("refused quote: %v", err)
	}
	f.oneclick.quoteErr = errors.New("connection refused")
	if _, err := f.svc.RequestQuote(ctx, escrowID, approverGuest, quoteInput); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("1Click down: %v", err)
	}
	f.oneclick.quoteErr = nil
	f.oneclick.quote.AmountOut = "1499999"
	if _, err := f.svc.RequestQuote(ctx, escrowID, approverGuest, quoteInput); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("other amount: %v", err)
	}
	if _, err := f.svc.RequestQuote(ctx, escrowID, approverGuest, funding.QuoteInput{OriginAsset: " "}); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("missing input: %v", err)
	}
}

func TestQuoteOnlyWhileCreated(t *testing.T) {
	ctx := context.Background()
	for _, st := range []domain.EscrowStatus{domain.EscrowOpen, domain.EscrowPendingApproval, domain.EscrowApproved} {
		f := newFixture(t, st)
		if st != domain.EscrowApproved {
			e := f.escrows.escrows[escrowID]
			e.Submission = nil
			f.escrows.escrows[escrowID] = e
		}
		if _, err := f.svc.RequestQuote(ctx, escrowID, approverGuest, quoteInput); !errors.Is(err, domain.ErrConflict) {
			t.Errorf("%s: %v", st, err)
		}
	}
	// Stored as Created, but the contract says Funded.
	f := newFixture(t, domain.EscrowCreated)
	f.escrows.chain[escrowID] = domain.EscrowFunded
	if _, err := f.svc.RequestQuote(ctx, escrowID, approverGuest, quoteInput); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("funded on-chain: %v", err)
	}
	if len(f.oneclick.requests) != 0 {
		t.Fatal("quote requested for an escrow that is not Created")
	}
}

func TestFundingWatcher(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t, domain.EscrowCreated)
	if _, err := f.svc.Status(ctx, escrowID, approverGuest); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("status before a quote: %v", err)
	}
	q, _ := f.svc.RequestQuote(ctx, escrowID, approverGuest, quoteInput)

	due, _ := f.svc.FindDue(ctx, time.Minute, 10)
	if !slices.Equal(due, []string{q.ID}) {
		t.Fatalf("due = %v", due)
	}
	f.oneclick.statusErr = funding.ErrDepositNotFound
	if err := f.svc.Check(ctx, q.ID); err != nil {
		t.Fatal(err)
	}
	f.oneclick.statusErr = nil
	for _, raw := range []string{"PENDING_DEPOSIT", "PROCESSING", "FAILED"} {
		f.oneclick.status = raw
		f.repo.quotes[0].Watching = true
		if err := f.svc.Check(ctx, q.ID); err != nil {
			t.Fatal(err)
		}
		got := f.repo.quotes[0]
		if got.Status != domain.FundingPending || got.OneClickStatus != raw || got.Watching != (raw != "FAILED") {
			t.Fatalf("%s: quote = %+v", raw, got)
		}
	}

	f.repo.quotes[0].Watching = true
	f.oneclick.status = "SUCCESS"
	f.escrows.chain[escrowID] = domain.EscrowFunded
	syncs := f.escrows.syncs
	if err := f.svc.Check(ctx, q.ID); err != nil {
		t.Fatal(err)
	}
	if got := f.repo.quotes[0]; got.Status != domain.FundingSuccess || got.Watching || f.escrows.syncs != syncs+1 {
		t.Fatalf("after SUCCESS: %+v, syncs %d", got, f.escrows.syncs)
	}
	// Watching stopped: checking again does nothing.
	f.oneclick.status = "REFUNDED"
	_ = f.svc.Check(ctx, q.ID)
	if f.repo.quotes[0].Status != domain.FundingSuccess {
		t.Fatal("a stopped quote was checked again")
	}
	v, err := f.svc.Status(ctx, escrowID, approverGuest)
	if err != nil || v.Status != domain.FundingSuccess || v.EscrowStatus != domain.EscrowFunded {
		t.Fatalf("Status = %+v, %v", v, err)
	}
	if _, err := f.svc.Status(ctx, escrowID, domain.Requester{AccountID: "acct-freelancer"}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("status by the freelancer: %v", err)
	}
}

func TestFundingWatcherStopsAfterGrace(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t, domain.EscrowCreated)
	q, _ := f.svc.RequestQuote(ctx, escrowID, approverGuest, quoteInput)
	f.oneclick.status = "PENDING_DEPOSIT"
	f.now = start.Add(time.Hour + 24*time.Hour + time.Second)
	if err := f.svc.Check(ctx, q.ID); err != nil {
		t.Fatal(err)
	}
	if got := f.repo.quotes[0]; got.Watching || got.Status != domain.FundingPending {
		t.Fatalf("quote = %+v", got)
	}
}

func TestStatusRefreshesFromOneClick(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t, domain.EscrowCreated)
	_, _ = f.svc.RequestQuote(ctx, escrowID, approverGuest, quoteInput)
	f.oneclick.status = "REFUNDED"
	v, err := f.svc.Status(ctx, escrowID, approverGuest)
	if err != nil || v.Status != domain.FundingRefunded || v.Quote.OneClickStatus != "REFUNDED" {
		t.Fatalf("Status = %+v, %v", v, err)
	}
	// 1Click down: the stored status is shown.
	f.repo.quotes[0].Watching = true
	f.oneclick.statusErr = errors.New("timeout")
	if v, err := f.svc.Status(ctx, escrowID, approverGuest); err != nil || v.Status != domain.FundingRefunded {
		t.Fatalf("Status with 1Click down = %+v, %v", v, err)
	}
}

func TestTokensAreCached(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t, domain.EscrowCreated)
	f.oneclick.tokens = []domain.Token{{AssetID: "nep141:wrap.near", Symbol: "wNEAR"}}
	for range 3 {
		if tokens, err := f.svc.Tokens(ctx); err != nil || len(tokens) != 1 {
			t.Fatalf("Tokens = %v, %v", tokens, err)
		}
	}
	if f.oneclick.tokenCalls != 1 {
		t.Fatalf("1Click called %d times", f.oneclick.tokenCalls)
	}
	f.now = start.Add(6 * time.Minute)
	f.oneclick.tokenErr = errors.New("down")
	if tokens, err := f.svc.Tokens(ctx); err != nil || len(tokens) != 1 {
		t.Fatalf("stale list not served: %v", err)
	}
	empty := newFixture(t, domain.EscrowCreated)
	empty.oneclick.tokenErr = errors.New("down")
	if _, err := empty.svc.Tokens(ctx); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("no list: %v", err)
	}
}
