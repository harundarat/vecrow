package http_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/harundarat/vecrow/backend/internal/domain"
	"github.com/harundarat/vecrow/backend/internal/usecase/auth"
	"github.com/harundarat/vecrow/backend/internal/usecase/escrow"
	"github.com/harundarat/vecrow/backend/internal/usecase/funding"
)

const (
	validGuestToken = "guest-token-0123456789abcdefghijklmnopqrstu"
	testEscrowID    = "0b7d2c1e-6a1f-4f7a-9d55-2f1c4e7b9a10"
	explorerTxURL   = "https://explorer.example/txns/"
)

var testGuest = domain.GuestToken{
	EscrowID:  testEscrowID,
	Wallet:    domain.Wallet{Scheme: domain.SchemeNEP413, Address: "client.near"},
	ExpiresAt: time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC),
}

// fakeGuests accepts validGuestToken.
type fakeGuests struct{}

func (fakeGuests) Authenticate(_ context.Context, token string) (domain.GuestToken, error) {
	if token != validGuestToken {
		return domain.GuestToken{}, domain.Unauthorized("guest token is invalid or expired")
	}
	return testGuest, nil
}

// fakeEscrows records the last call and returns the configured results.
type fakeEscrows struct {
	escrow domain.Escrow
	view   escrow.View
	action escrow.ActionResult
	list   []escrow.AccountEscrow
	err    error

	method    string
	id        string
	accountID string
	requester domain.Requester
	create    escrow.CreateInput
	details   escrow.DetailsInput
	submit    escrow.SubmitInput
	cancel    escrow.CancelInput
	approver  escrow.ApproverAction
	delegate  []byte
	milestone uint32
	reason    string
	wallet    domain.Wallet
	filters   [2]string
	invite    *escrow.InvitationInput
}

func (f *fakeEscrows) ContractID() string    { return "escrow.test.near" }
func (f *fakeEscrows) Link(id string) string { return "https://app.example/escrows/" + id }

func (f *fakeEscrows) Create(_ context.Context, in escrow.CreateInput) (domain.Escrow, error) {
	f.method, f.create = "Create", in
	return f.escrow, f.err
}

func (f *fakeEscrows) Get(_ context.Context, id string, req domain.Requester) (escrow.View, error) {
	f.method, f.id, f.requester = "Get", id, req
	return f.view, f.err
}

func (f *fakeEscrows) BuildTerms(_ context.Context, id string, in escrow.DetailsInput) (escrow.Terms, error) {
	f.method, f.id, f.details = "BuildTerms", id, in
	return escrow.Terms{Message: "terms message", Nonce: "ab", Recipient: "escrow.test.near"}, f.err
}

func (f *fakeEscrows) SubmitCounterparty(_ context.Context, id string, in escrow.SubmitInput) (escrow.SubmitResult, error) {
	f.method, f.id, f.submit = "SubmitCounterparty", id, in
	return escrow.SubmitResult{Escrow: f.escrow, GuestToken: validGuestToken, GuestExpiresAt: testGuest.ExpiresAt}, f.err
}

func (f *fakeEscrows) Approve(_ context.Context, accountID, id string, in escrow.ApproveInput) (domain.Escrow, error) {
	f.method, f.accountID, f.id = "Approve", accountID, id
	return f.escrow, f.err
}

func (f *fakeEscrows) Reject(_ context.Context, accountID, id string) (domain.Escrow, error) {
	f.method, f.accountID, f.id = "Reject", accountID, id
	return f.escrow, f.err
}

func (f *fakeEscrows) Cancel(_ context.Context, id string, in escrow.CancelInput) (escrow.ActionResult, error) {
	f.method, f.id, f.cancel = "Cancel", id, in
	return f.action, f.err
}

func (f *fakeEscrows) RenewGuestToken(_ context.Context, id string, w domain.Wallet) (string, time.Time, error) {
	f.method, f.id, f.wallet = "RenewGuestToken", id, w
	return validGuestToken, testGuest.ExpiresAt, f.err
}

func (f *fakeEscrows) SubmitMilestone(_ context.Context, id string, n uint32, delegate []byte) (escrow.ActionResult, error) {
	f.method, f.id, f.milestone, f.delegate = "SubmitMilestone", id, n, delegate
	return f.action, f.err
}

func (f *fakeEscrows) ApproveMilestone(_ context.Context, id string, n uint32, a escrow.ApproverAction) (escrow.ActionResult, error) {
	f.method, f.id, f.milestone, f.approver = "ApproveMilestone", id, n, a
	return f.action, f.err
}

func (f *fakeEscrows) RequestRevision(_ context.Context, id string, n uint32, a escrow.ApproverAction, reason string) (escrow.ActionResult, error) {
	f.method, f.id, f.milestone, f.approver, f.reason = "RequestRevision", id, n, a, reason
	return f.action, f.err
}

func (f *fakeEscrows) RefundUnsubmitted(_ context.Context, id string, delegate []byte) (escrow.ActionResult, error) {
	f.method, f.id, f.delegate = "RefundUnsubmitted", id, delegate
	return f.action, f.err
}

func (f *fakeEscrows) UpdateInvitation(_ context.Context, accountID, id string, in *escrow.InvitationInput) (domain.Escrow, error) {
	f.method, f.accountID, f.id, f.invite = "UpdateInvitation", accountID, id, in
	return f.escrow, f.err
}

func (f *fakeEscrows) ListMine(_ context.Context, accountID, role, status string) ([]escrow.AccountEscrow, error) {
	f.method, f.accountID, f.filters = "ListMine", accountID, [2]string{role, status}
	return f.list, f.err
}

// fakeProver proves the wallet client.near for nonce "n1".
type fakeProver struct{}

func (fakeProver) Prove(_ context.Context, p auth.Proof) (domain.Wallet, error) {
	if p.Nonce != "n1" {
		return domain.Wallet{}, domain.Unauthorized("invalid signature")
	}
	return testGuest.Wallet, nil
}

type fakeFunding struct {
	tokens    []domain.Token
	quote     domain.FundingQuote
	view      funding.View
	err       error
	requester domain.Requester
	input     funding.QuoteInput
}

func (f *fakeFunding) Tokens(context.Context) ([]domain.Token, error) { return f.tokens, f.err }

func (f *fakeFunding) RequestQuote(_ context.Context, _ string, req domain.Requester, in funding.QuoteInput) (domain.FundingQuote, error) {
	f.requester, f.input = req, in
	return f.quote, f.err
}

func (f *fakeFunding) Status(_ context.Context, _ string, req domain.Requester) (funding.View, error) {
	f.requester = req
	return f.view, f.err
}

func ptr[T any](v T) *T { return &v }

// openEscrow is an escrow created by a freelancer, with a pending
// submission from the client.
func openEscrow() domain.Escrow {
	terms := domain.EscrowTerms{
		EscrowID:   testEscrowID,
		Freelancer: "freelancer.near",
		Milestones: []string{"1000000", "2000000"},
		Split:      []domain.SplitShare{{AccountID: "freelancer.near", BPS: 10_000}},
		Approver: domain.Approver{
			Scheme: domain.SchemeNEP413, AccountID: "client.near",
			PublicKey: "ed25519:DcA2MzgpJbrUATQLLceocVckhhAqrkingax4oJ9kZ847",
		},
		RefundAccount: "client.near",
		FeeBPS:        50,
		DeadlineMS:    1_800_000_000_000,
	}
	return domain.Escrow{
		ID: testEscrowID, CreatorAccountID: "acct-1", CreatorRole: domain.RoleFreelancer,
		Status: domain.EscrowPendingApproval, FeeBPS: 50,
		Milestones: []domain.Milestone{
			{Amount: "1000000", Status: domain.MilestonePending},
			{Amount: "2000000", Status: domain.MilestonePending},
		},
		Freelancer: &domain.FreelancerDetails{Account: "freelancer.near", Split: terms.Split},
		Submission: &domain.Submission{
			ID: "sub-1", Status: domain.SubmissionPending, Terms: terms,
			Message: terms.Message("escrow.test.near"), Wallet: testGuest.Wallet,
		},
		CreatedAt: time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC),
	}
}

func decode(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("invalid JSON %s: %v", body, err)
	}
	return m
}

func TestCreateEscrow(t *testing.T) {
	srv := newTestServer(t, fakeIssuer{})
	srv.escrows.escrow = openEscrow()
	body := `{"role":"freelancer","milestones":["1000000","2000000"],"freelancer":"freelancer.near",
		"split":[{"account_id":"freelancer.near","bps":10000}]}`

	if rec := srv.do(http.MethodPost, "/api/v1/escrows", body); rec.Code != http.StatusUnauthorized {
		t.Fatalf("without session: status %d", rec.Code)
	}
	rec := srv.do(http.MethodPost, "/api/v1/escrows", body, "Authorization", bearer)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d body %s", rec.Code, rec.Body)
	}
	in := srv.escrows.create
	if in.AccountID != "acct-1" || in.Role != "freelancer" || len(in.Milestones) != 2 ||
		in.Details.Freelancer == nil || in.Details.Client != nil ||
		in.Details.Freelancer.Split[0] != (domain.SplitShare{AccountID: "freelancer.near", BPS: 10_000}) {
		t.Errorf("input = %+v", in)
	}
	m := decode(t, rec.Body.Bytes())
	if m["link"] != "https://app.example/escrows/"+testEscrowID || m["contract_id"] != "escrow.test.near" ||
		m["status"] != "PendingApproval" || m["total"] != "3000000" {
		t.Errorf("body = %v", m)
	}
	sub, _ := m["submission"].(map[string]any)
	if sub == nil || sub["recipient"] != "escrow.test.near" || len(sub["nonce"].(string)) != 64 ||
		!strings.HasPrefix(sub["message"].(string), "Vecrow create_escrow\n") {
		t.Errorf("submission = %v", sub)
	}

	if rec := srv.do(http.MethodPost, "/api/v1/escrows", `{"milestones":["1"]}`, "Authorization", bearer); rec.Code != http.StatusBadRequest {
		t.Errorf("missing role: status %d", rec.Code)
	}
}

func TestGetEscrowShowsTransactionsAndRequester(t *testing.T) {
	srv := newTestServer(t, fakeIssuer{})
	e := openEscrow()
	e.Status = domain.EscrowFunded
	e.ReviewWindowSec = ptr(uint64(1_209_600))
	e.Milestones[0] = domain.Milestone{Amount: "1000000", Status: domain.MilestoneSubmitted, SubmittedAtMS: ptr(uint64(1_000))}
	e.PendingPayouts = []domain.PendingPayout{{AccountID: "member.near", Amount: "995000"}}
	srv.escrows.view = escrow.View{
		Escrow: e,
		Transactions: []domain.Transaction{{
			Hash: "9xT", Method: "submit_milestone", SignerID: "relayer.near", SenderID: "freelancer.near",
			ReceiverID: "escrow.test.near", Succeeded: true,
			Events: []domain.ContractEvent{{Event: "milestone_submitted", Data: []byte(`{"escrow_id":"x","milestone":0}`)}},
		}},
		RevisionNotes: []domain.RevisionNote{{Milestone: 0, Reason: "fix the logo", TxHash: "8yU"}},
	}

	rec := srv.do(http.MethodGet, "/api/v1/escrows/"+testEscrowID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body)
	}
	if srv.escrows.id != testEscrowID || srv.escrows.requester.AccountID != "" || srv.escrows.requester.Guest != nil {
		t.Errorf("id %q requester %+v", srv.escrows.id, srv.escrows.requester)
	}
	m := decode(t, rec.Body.Bytes())
	txs := m["transactions"].([]any)
	tx := txs[0].(map[string]any)
	if tx["url"] != explorerTxURL+"9xT" || tx["sender_id"] != "freelancer.near" || tx["signer_id"] != "relayer.near" {
		t.Errorf("transaction = %v", tx)
	}
	ev := tx["events"].([]any)[0].(map[string]any)
	if ev["event"] != "milestone_submitted" || ev["data"].(map[string]any)["milestone"] != float64(0) {
		t.Errorf("event = %v", ev)
	}
	pending := m["pending_payouts"].([]any)[0].(map[string]any)
	if pending["account_id"] != "member.near" || pending["amount"] != "995000" {
		t.Errorf("pending payouts = %v", m["pending_payouts"])
	}
	ms := m["milestones"].([]any)[0].(map[string]any)
	if ms["auto_release_at_ms"] != float64(1_000+1_209_600_000) {
		t.Errorf("milestone = %v", ms)
	}
	if notes := m["revision_notes"].([]any); notes[0].(map[string]any)["reason"] != "fix the logo" {
		t.Errorf("revision notes = %v", notes)
	}

	rec = srv.do(http.MethodGet, "/api/v1/escrows/"+testEscrowID, "", "X-Guest-Token", validGuestToken, "Authorization", bearer)
	if rec.Code != http.StatusOK || srv.escrows.requester.AccountID != "acct-1" ||
		srv.escrows.requester.Guest == nil || *srv.escrows.requester.Guest != testGuest {
		t.Errorf("status %d requester %+v", rec.Code, srv.escrows.requester)
	}
	if rec := srv.do(http.MethodGet, "/api/v1/escrows/"+testEscrowID, "", "X-Guest-Token", "expired"); rec.Code != http.StatusUnauthorized {
		t.Errorf("invalid guest token: status %d", rec.Code)
	}
	if rec := srv.do(http.MethodGet, "/api/v1/escrows/"+testEscrowID, "", "Authorization", "Basic abc"); rec.Code != http.StatusUnauthorized {
		t.Errorf("non-bearer Authorization: status %d", rec.Code)
	}
}

func TestCounterpartyFlow(t *testing.T) {
	srv := newTestServer(t, fakeIssuer{})
	srv.escrows.escrow = openEscrow()
	details := `"approver":{"scheme":"nep413","account_id":"client.near","public_key":"ed25519:abc"},"refund_account":"client.near"`

	rec := srv.do(http.MethodPost, "/api/v1/escrows/"+testEscrowID+"/terms", "{"+details+"}")
	if rec.Code != http.StatusOK || srv.escrows.details.Client == nil || srv.escrows.details.Client.Approver.AccountID != "client.near" {
		t.Fatalf("terms: status %d body %s details %+v", rec.Code, rec.Body, srv.escrows.details)
	}
	if m := decode(t, rec.Body.Bytes()); m["message"] != "terms message" || m["recipient"] != "escrow.test.near" {
		t.Errorf("terms body = %v", m)
	}

	if rec := srv.do(http.MethodPost, "/api/v1/escrows/"+testEscrowID+"/counterparty", "{"+details+"}"); rec.Code != http.StatusBadRequest {
		t.Errorf("without signature: status %d", rec.Code)
	}
	rec = srv.do(http.MethodPost, "/api/v1/escrows/"+testEscrowID+"/counterparty",
		`{`+details+`,"deadline_ms":1800000000000,"signature":"c2ln","public_key":"ed25519:abc"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("counterparty: status %d body %s", rec.Code, rec.Body)
	}
	if in := srv.escrows.submit; in.DeadlineMS != 1_800_000_000_000 || in.Signature != "c2ln" || in.PublicKey != "ed25519:abc" {
		t.Errorf("submit input = %+v", in)
	}
	if m := decode(t, rec.Body.Bytes()); m["guest_token"] != validGuestToken || m["escrow"] == nil {
		t.Errorf("counterparty body = %v", m)
	}

	if rec := srv.do(http.MethodPost, "/api/v1/escrows/"+testEscrowID+"/counterparty/approve", `{"signature":"c2ln"}`); rec.Code != http.StatusUnauthorized {
		t.Errorf("approve without session: status %d", rec.Code)
	}
	rec = srv.do(http.MethodPost, "/api/v1/escrows/"+testEscrowID+"/counterparty/approve", `{"signature":"c2ln"}`, "Authorization", bearer)
	if rec.Code != http.StatusOK || srv.escrows.method != "Approve" || srv.escrows.accountID != "acct-1" {
		t.Errorf("approve: status %d method %s account %s", rec.Code, srv.escrows.method, srv.escrows.accountID)
	}
	rec = srv.do(http.MethodPost, "/api/v1/escrows/"+testEscrowID+"/counterparty/reject", "", "Authorization", bearer)
	if rec.Code != http.StatusOK || srv.escrows.method != "Reject" {
		t.Errorf("reject: status %d method %s", rec.Code, srv.escrows.method)
	}

	rec = srv.do(http.MethodPost, "/api/v1/escrows/"+testEscrowID+"/guest-token",
		`{"nonce":"n1","signature":"sig","public_key":"ed25519:abc"}`)
	if rec.Code != http.StatusCreated || srv.escrows.wallet != testGuest.Wallet {
		t.Errorf("guest token: status %d wallet %+v", rec.Code, srv.escrows.wallet)
	}
	rec = srv.do(http.MethodPost, "/api/v1/escrows/"+testEscrowID+"/guest-token",
		`{"nonce":"other","signature":"sig","public_key":"ed25519:abc"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("guest token with a bad proof: status %d", rec.Code)
	}
}

func TestRelayedActions(t *testing.T) {
	srv := newTestServer(t, fakeIssuer{})
	srv.escrows.action = escrow.ActionResult{
		Escrow:      openEscrow(),
		Transaction: domain.Transaction{Hash: "7zV", Method: "submit_milestone", Succeeded: true},
	}
	delegate := base64.StdEncoding.EncodeToString([]byte{1, 2, 3})

	rec := srv.do(http.MethodPost, "/api/v1/escrows/"+testEscrowID+"/milestones/1/submit", `{"delegate_action":"`+delegate+`"}`)
	if rec.Code != http.StatusOK || srv.escrows.milestone != 1 || string(srv.escrows.delegate) != "\x01\x02\x03" {
		t.Fatalf("submit: status %d body %s", rec.Code, rec.Body)
	}
	if tx := decode(t, rec.Body.Bytes())["transaction"].(map[string]any); tx["hash"] != "7zV" || tx["url"] != explorerTxURL+"7zV" {
		t.Errorf("transaction = %v", tx)
	}
	for _, path := range []string{"/milestones/x/submit", "/milestones/-1/submit", "/milestones/99/submit"} {
		if rec := srv.do(http.MethodPost, "/api/v1/escrows/"+testEscrowID+path, `{"delegate_action":"`+delegate+`"}`); rec.Code != http.StatusNotFound {
			t.Errorf("%s: status %d", path, rec.Code)
		}
	}
	if rec := srv.do(http.MethodPost, "/api/v1/escrows/"+testEscrowID+"/milestones/0/submit", `{"delegate_action":"%%%"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("bad base64: status %d", rec.Code)
	}

	action := `"nonce":"` + strings.Repeat("ab", 32) + `","deadline_ms":1800000000000,"signature":"c2ln"`
	rec = srv.do(http.MethodPost, "/api/v1/escrows/"+testEscrowID+"/milestones/0/approve", "{"+action+"}")
	if rec.Code != http.StatusOK || srv.escrows.method != "ApproveMilestone" || srv.escrows.approver.DeadlineMS != 1_800_000_000_000 {
		t.Errorf("approve: status %d method %s action %+v", rec.Code, srv.escrows.method, srv.escrows.approver)
	}
	if rec := srv.do(http.MethodPost, "/api/v1/escrows/"+testEscrowID+"/milestones/0/revision", "{"+action+"}"); rec.Code != http.StatusBadRequest {
		t.Errorf("revision without reason: status %d", rec.Code)
	}
	rec = srv.do(http.MethodPost, "/api/v1/escrows/"+testEscrowID+"/milestones/0/revision", "{"+action+`,"reason":"fix it"}`)
	if rec.Code != http.StatusOK || srv.escrows.reason != "fix it" {
		t.Errorf("revision: status %d reason %q", rec.Code, srv.escrows.reason)
	}
	rec = srv.do(http.MethodPost, "/api/v1/escrows/"+testEscrowID+"/refund", `{"delegate_action":"`+delegate+`"}`)
	if rec.Code != http.StatusOK || srv.escrows.method != "RefundUnsubmitted" {
		t.Errorf("refund: status %d method %s", rec.Code, srv.escrows.method)
	}

	srv.escrows.err = domain.Unavailable("the transaction outcome is unknown")
	if rec := srv.do(http.MethodPost, "/api/v1/escrows/"+testEscrowID+"/refund", `{"delegate_action":"`+delegate+`"}`); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("unavailable: status %d", rec.Code)
	}
}

func TestCancel(t *testing.T) {
	srv := newTestServer(t, fakeIssuer{})
	srv.escrows.action = escrow.ActionResult{Escrow: openEscrow()}

	rec := srv.do(http.MethodPost, "/api/v1/escrows/"+testEscrowID+"/cancel", "", "Authorization", bearer)
	if rec.Code != http.StatusOK || srv.escrows.cancel.AccountID != "acct-1" || srv.escrows.cancel.Approver != nil || srv.escrows.cancel.Delegate != nil {
		t.Fatalf("by session: status %d input %+v", rec.Code, srv.escrows.cancel)
	}
	if m := decode(t, rec.Body.Bytes()); m["transaction"] != nil {
		t.Errorf("off-chain cancel has a transaction: %v", m["transaction"])
	}
	rec = srv.do(http.MethodPost, "/api/v1/escrows/"+testEscrowID+"/cancel",
		`{"nonce":"`+strings.Repeat("cd", 32)+`","deadline_ms":5,"signature":"c2ln"}`)
	if rec.Code != http.StatusOK || srv.escrows.cancel.AccountID != "" || srv.escrows.cancel.Approver == nil ||
		srv.escrows.cancel.Approver.DeadlineMS != 5 {
		t.Errorf("by approver: status %d input %+v", rec.Code, srv.escrows.cancel)
	}
	rec = srv.do(http.MethodPost, "/api/v1/escrows/"+testEscrowID+"/cancel", `{"delegate_action":"AQID"}`)
	if rec.Code != http.StatusOK || string(srv.escrows.cancel.Delegate) != "\x01\x02\x03" {
		t.Errorf("by freelancer: status %d input %+v", rec.Code, srv.escrows.cancel)
	}
}

func TestListMyEscrows(t *testing.T) {
	srv := newTestServer(t, fakeIssuer{})
	srv.escrows.list = []escrow.AccountEscrow{{Escrow: openEscrow(), Roles: []domain.Role{domain.RoleFreelancer, domain.RoleMember}}}

	if rec := srv.do(http.MethodGet, "/api/v1/me/escrows", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("without session: status %d", rec.Code)
	}
	rec := srv.do(http.MethodGet, "/api/v1/me/escrows?role=member&status=Funded", "", "Authorization", bearer)
	if rec.Code != http.StatusOK || srv.escrows.accountID != "acct-1" || srv.escrows.filters != [2]string{"member", "Funded"} {
		t.Fatalf("status %d account %s filters %v", rec.Code, srv.escrows.accountID, srv.escrows.filters)
	}
	list := decode(t, rec.Body.Bytes())["escrows"].([]any)
	roles := list[0].(map[string]any)["roles"].([]any)
	if len(list) != 1 || len(roles) != 2 || roles[1] != "member" {
		t.Errorf("escrows = %v", list)
	}
}

func TestFundingRoutes(t *testing.T) {
	srv := newTestServer(t, fakeIssuer{})
	srv.funding.tokens = []domain.Token{{AssetID: "nep141:usdc.near", Symbol: "USDC", Blockchain: "near", Decimals: 6, Price: 1}}
	srv.funding.quote = domain.FundingQuote{ID: "q1", DepositAddress: "0xdeposit", AmountIn: "3010000", AmountOut: "3000000", Status: domain.FundingPending}
	srv.funding.view = funding.View{
		Status: domain.FundingPending, EscrowStatus: domain.EscrowCreated,
		Quote: domain.FundingQuote{ID: "q1", Status: domain.FundingPending, OneClickStatus: "FAILED"},
	}

	rec := srv.do(http.MethodGet, "/api/v1/tokens", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("tokens: status %d", rec.Code)
	}
	if tok := decode(t, rec.Body.Bytes())["tokens"].([]any)[0].(map[string]any); tok["asset_id"] != "nep141:usdc.near" || tok["decimals"] != float64(6) {
		t.Errorf("tokens = %v", tok)
	}

	rec = srv.do(http.MethodPost, "/api/v1/escrows/"+testEscrowID+"/quote",
		`{"origin_asset":"nep141:eth.omft.near","refund_to":"0xabc"}`, "X-Guest-Token", validGuestToken)
	if rec.Code != http.StatusCreated || srv.funding.requester.Guest == nil ||
		srv.funding.input != (funding.QuoteInput{OriginAsset: "nep141:eth.omft.near", RefundTo: "0xabc"}) {
		t.Fatalf("quote: status %d body %s requester %+v", rec.Code, rec.Body, srv.funding.requester)
	}
	if q := decode(t, rec.Body.Bytes()); q["deposit_address"] != "0xdeposit" || q["amount_out"] != "3000000" {
		t.Errorf("quote = %v", q)
	}
	if rec := srv.do(http.MethodPost, "/api/v1/escrows/"+testEscrowID+"/quote", `{"origin_asset":"x"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("quote without refund_to: status %d", rec.Code)
	}

	rec = srv.do(http.MethodGet, "/api/v1/escrows/"+testEscrowID+"/funding", "", "Authorization", bearer)
	if rec.Code != http.StatusOK || srv.funding.requester.AccountID != "acct-1" {
		t.Fatalf("funding: status %d", rec.Code)
	}
	if m := decode(t, rec.Body.Bytes()); m["status"] != "PENDING" || m["oneclick_status"] != "FAILED" || m["escrow_status"] != "Created" {
		t.Errorf("funding = %v", m)
	}
}

func TestCORSAllowsGuestTokenHeader(t *testing.T) {
	srv := newTestServer(t, fakeIssuer{})
	rec := srv.do(http.MethodOptions, "/api/v1/escrows/"+testEscrowID, "",
		"Origin", "http://localhost:5173",
		"Access-Control-Request-Method", "GET",
		"Access-Control-Request-Headers", "x-guest-token")
	if allowed := rec.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(strings.ToLower(allowed), "x-guest-token") {
		t.Errorf("status %d allowed headers %q", rec.Code, allowed)
	}
}

func TestGuestTokenIsNotLogged(t *testing.T) {
	srv := newTestServer(t, fakeIssuer{})
	srv.do(http.MethodGet, "/api/v1/escrows/"+testEscrowID, "", "X-Guest-Token", validGuestToken)
	if strings.Contains(srv.logs.String(), validGuestToken) {
		t.Errorf("guest token logged: %s", srv.logs)
	}
}

func TestCreateEscrowWithInvitation(t *testing.T) {
	srv := newTestServer(t, fakeIssuer{})
	e := openEscrow()
	e.Invitation = &domain.Invitation{Kind: domain.InviteByEmail, Email: "bob@example.com"}
	srv.escrows.escrow = e
	body := `{"role":"freelancer","milestones":["1"],"freelancer":"freelancer.near",
		"split":[{"account_id":"freelancer.near","bps":10000}],
		"invitation":{"wallet":{"scheme":"eip191","address":"0xabc"}}}`
	rec := srv.do(http.MethodPost, "/api/v1/escrows", body, "Authorization", bearer)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d body %s", rec.Code, rec.Body)
	}
	in := srv.escrows.create.Invitation
	if in == nil || in.Wallet == nil || *in.Wallet != (escrow.WalletInput{Scheme: "eip191", Address: "0xabc"}) || in.Email != "" {
		t.Fatalf("invitation input = %+v", in)
	}
	inv := decode(t, rec.Body.Bytes())["invitation"].(map[string]any)
	if inv["kind"] != "email" || inv["email"] != "bob@example.com" || inv["wallet"] != nil || inv["username"] != nil {
		t.Fatalf("invitation = %v", inv)
	}
}

func TestUpdateInvitation(t *testing.T) {
	srv := newTestServer(t, fakeIssuer{})
	e := openEscrow()
	e.Invitation = &domain.Invitation{Kind: domain.InviteByUsername, AccountID: "acct-2", Username: "bob"}
	srv.escrows.escrow = e
	path := "/api/v1/escrows/" + testEscrowID + "/invitation"

	if rec := srv.do(http.MethodPatch, path, `{"invitation":{"username":"bob"}}`); rec.Code != http.StatusUnauthorized {
		t.Fatalf("without session: status %d", rec.Code)
	}
	if rec := srv.do(http.MethodPatch, path, `{"invitation":{"username":"bob"}}`, "Authorization", recoveryBearer); rec.Code != http.StatusForbidden {
		t.Fatalf("recovery session: status %d", rec.Code)
	}
	if srv.escrows.method != "" {
		t.Fatalf("the use case ran: %s", srv.escrows.method)
	}

	rec := srv.do(http.MethodPatch, path, `{"invitation":{"username":"bob"}}`, "Authorization", bearer)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body)
	}
	if srv.escrows.accountID != "acct-1" || srv.escrows.id != testEscrowID || srv.escrows.invite == nil || srv.escrows.invite.Username != "bob" {
		t.Fatalf("call = %s %s %+v", srv.escrows.accountID, srv.escrows.id, srv.escrows.invite)
	}
	m := decode(t, rec.Body.Bytes())
	if inv := m["invitation"].(map[string]any); inv["kind"] != "username" || inv["username"] != "bob" || inv["account_id"] != nil {
		t.Fatalf("invitation = %v", inv)
	}

	srv.escrows.escrow.Invitation = nil
	rec = srv.do(http.MethodPatch, path, `{"invitation":null}`, "Authorization", bearer)
	if rec.Code != http.StatusOK || srv.escrows.invite != nil {
		t.Fatalf("remove: status %d, input %+v", rec.Code, srv.escrows.invite)
	}
	if m := decode(t, rec.Body.Bytes()); m["invitation"] != nil {
		t.Fatalf("invitation after removal = %v", m["invitation"])
	}
	for _, body := range []string{`{}`, `{"invitation":"bob"}`, `{"invitation":[1]}`} {
		srv.escrows.method = ""
		if rec := srv.do(http.MethodPatch, path, body, "Authorization", bearer); rec.Code != http.StatusBadRequest || srv.escrows.method != "" {
			t.Errorf("%s: status %d, method %q", body, rec.Code, srv.escrows.method)
		}
	}
	srv.escrows.err = domain.Conflict("escrow is Created")
	if rec := srv.do(http.MethodPatch, path, `{"invitation":null}`, "Authorization", bearer); rec.Code != http.StatusConflict {
		t.Errorf("conflict: status %d", rec.Code)
	}
}

func TestGetEscrowShowsViewerInvitationAndWarnings(t *testing.T) {
	srv := newTestServer(t, fakeIssuer{})
	e := openEscrow()
	e.Invitation = &domain.Invitation{Kind: domain.InviteByEmail, Email: "bob@example.com"}

	// The invitation is in the escrow but not shown unless the view says so.
	srv.escrows.view = escrow.View{Escrow: e}
	m := decode(t, srv.do(http.MethodGet, "/api/v1/escrows/"+testEscrowID, "").Body.Bytes())
	if m["invitation"] != nil || len(m["warnings"].([]any)) != 0 {
		t.Fatalf("anonymous body: invitation %v, warnings %v", m["invitation"], m["warnings"])
	}
	if v := m["viewer"].(map[string]any); v["creator"] != false || v["invited"] != false {
		t.Fatalf("viewer = %v", v)
	}

	srv.escrows.view = escrow.View{
		Escrow: e, Creator: true, Invitation: e.Invitation,
		Warnings: []escrow.Warning{{Code: escrow.WarningSubmitterNotInvited, Message: "check before you approve"}},
	}
	m = decode(t, srv.do(http.MethodGet, "/api/v1/escrows/"+testEscrowID, "", "Authorization", bearer).Body.Bytes())
	if inv := m["invitation"].(map[string]any); inv["email"] != "bob@example.com" {
		t.Fatalf("invitation = %v", inv)
	}
	w := m["warnings"].([]any)[0].(map[string]any)
	if w["code"] != "submitter_not_invited" || w["message"] != "check before you approve" {
		t.Fatalf("warnings = %v", m["warnings"])
	}
	if v := m["viewer"].(map[string]any); v["creator"] != true || v["invited"] != false {
		t.Fatalf("viewer = %v", v)
	}
	// Other responses never carry the invitation.
	srv.escrows.escrow = e
	rec := srv.do(http.MethodPost, "/api/v1/escrows/"+testEscrowID+"/counterparty/reject", "", "Authorization", bearer)
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "bob@example.com") {
		t.Fatalf("reject: status %d body %s", rec.Code, rec.Body)
	}
}

func TestListMyEscrowsMarksInvitations(t *testing.T) {
	srv := newTestServer(t, fakeIssuer{})
	e := openEscrow()
	e.Invitation = &domain.Invitation{Kind: domain.InviteByEmail, Email: "bob@example.com"}
	srv.escrows.list = []escrow.AccountEscrow{{Escrow: e, Roles: []domain.Role{domain.RoleClient}, Invited: true}}
	rec := srv.do(http.MethodGet, "/api/v1/me/escrows?role=client", "", "Authorization", bearer)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	item := decode(t, rec.Body.Bytes())["escrows"].([]any)[0].(map[string]any)
	if item["invited"] != true || item["roles"].([]any)[0] != "client" || strings.Contains(rec.Body.String(), "bob@example.com") {
		t.Fatalf("item = %v", item)
	}
}
