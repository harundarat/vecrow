package repository_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"testing"

	"github.com/harundarat/vecrow/backend/internal/domain"
	"github.com/harundarat/vecrow/backend/internal/infrastructure/nearrpc"
	"github.com/harundarat/vecrow/backend/internal/neartx"
	"github.com/harundarat/vecrow/backend/internal/repository"
	"github.com/harundarat/vecrow/backend/internal/usecase/escrow"
	"github.com/harundarat/vecrow/backend/internal/walletsig/walletsigtest"
)

type sentTx struct {
	receiver string
	actions  []neartx.Action
}

// fakeSender records transactions and answers with outcome.
type fakeSender struct {
	account string
	sent    []sentTx
	outcome string
	err     error
}

func (s *fakeSender) AccountID() string { return s.account }

func (s *fakeSender) Send(_ context.Context, receiver string, actions ...neartx.Action) (*nearrpc.Outcome, error) {
	s.sent = append(s.sent, sentTx{receiver, actions})
	if s.err != nil {
		return nil, s.err
	}
	var out nearrpc.Outcome
	if err := json.Unmarshal([]byte(s.outcome), &out); err != nil {
		panic(err)
	}
	return &out, nil
}

// fakeViews answers view calls by method.
type fakeViews struct {
	results map[string]string
	calls   []string
}

func (v *fakeViews) CallFunction(_ context.Context, contract, method string, args []byte) ([]byte, uint64, error) {
	v.calls = append(v.calls, contract+"."+method+string(args))
	r, ok := v.results[method]
	if !ok {
		return nil, 0, errors.New("unexpected view " + method)
	}
	return []byte(r), 77, nil
}

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

// outcome is a direct call on contract that logs an event and returns value.
func outcome(contract, value string, logs ...string) string {
	l, _ := json.Marshal(logs)
	return `{"transaction":{"hash":"TXHASH"},
	  "transaction_outcome":{"id":"TXHASH","outcome":{"logs":[],"receipt_ids":["R1"],"status":{"SuccessReceiptId":"R1"},"executor_id":"relayer.test.near"}},
	  "receipts_outcome":[{"id":"R1","outcome":{"logs":` + string(l) + `,"receipt_ids":[],"status":{"SuccessValue":"` + b64(value) + `"},"executor_id":"` + contract + `"}}]}`
}

func TestEscrowContractCreateEscrow(t *testing.T) {
	approver, _ := domain.NewApprover(domain.SchemeEIP191, "", "", "0x7E5F4552091A69125d5DfCb7b8C2659029395Bdf")
	terms := domain.EscrowTerms{
		EscrowID: "e1", Freelancer: "alice.test.near", Milestones: []string{"10"},
		Split: []domain.SplitShare{{AccountID: "alice.test.near", BPS: 10000}}, Approver: approver,
		RefundAccount: "client.test.near", FeeBPS: 50, DeadlineMS: 99,
	}
	sender := &fakeSender{account: "relayer.test.near", outcome: outcome("escrow.test.near", "",
		`EVENT_JSON:{"standard":"vecrow","version":"1.0.0","event":"escrow_created","data":{"escrow_id":"e1"}}`,
		`EVENT_JSON:{"standard":"nep141","version":"1.0.0","event":"ft_transfer","data":[]}`)}
	c := repository.NewEscrowContract("escrow.test.near", &fakeViews{}, sender)

	tx, err := c.CreateEscrow(context.Background(), terms,
		domain.FreelancerSignature{PublicKey: "ed25519:PK", Signature: "FSIG"}, "0xCSIG")
	if err != nil {
		t.Fatal(err)
	}
	if len(sender.sent) != 1 || sender.sent[0].receiver != "escrow.test.near" {
		t.Fatalf("sent = %+v", sender.sent)
	}
	fc := sender.sent[0].actions[0].(neartx.FunctionCall)
	const want = `{"client_signature":"0xCSIG","freelancer_signature":{"public_key":"ed25519:PK","signature":"FSIG"},` +
		`"terms":{"escrow_id":"e1","freelancer":"alice.test.near","milestones":["10"],"split":[{"account_id":"alice.test.near","bps":10000}],` +
		`"approver":{"scheme":"eip191","address":"0x7e5f4552091a69125d5dfcb7b8c2659029395bdf"},"refund_account":"client.test.near","fee_bps":50,"deadline_ms":99}}`
	if fc.MethodName != "create_escrow" || string(fc.Args) != want || fc.Deposit.Sign() != 0 {
		t.Fatalf("call = %s %s deposit %v", fc.MethodName, fc.Args, fc.Deposit)
	}
	if !tx.Succeeded || tx.Hash != "TXHASH" || tx.Method != "create_escrow" || tx.SignerID != "relayer.test.near" ||
		tx.SenderID != "relayer.test.near" || tx.EscrowID != "e1" {
		t.Fatalf("tx = %+v", tx)
	}
	if len(tx.Events) != 1 || tx.Events[0].Event != "escrow_created" || tx.Events[0].ReceiptID != "R1" || tx.Events[0].LogIndex != 0 {
		t.Fatalf("events = %+v", tx.Events)
	}

	if _, err := c.CreateEscrow(context.Background(), terms, domain.FreelancerSignature{PublicKey: "ed25519:PK"}, "0xCSIG"); err == nil {
		t.Fatal("create_escrow sent without the freelancer's signature")
	}
	if len(sender.sent) != 1 {
		t.Fatal("a transaction was sent without both signatures")
	}
}

func TestEscrowContractSignedCalls(t *testing.T) {
	sender := &fakeSender{account: "relayer.test.near", outcome: outcome("escrow.test.near", "")}
	c := repository.NewEscrowContract("escrow.test.near", &fakeViews{}, sender)
	a := escrow.ApproverAction{Nonce: strings.Repeat("ab", 32), DeadlineMS: 1767225600000, Signature: "SIG"}
	ctx := context.Background()
	_, _ = c.ApproveMilestone(ctx, "e1", 2, a)
	_, _ = c.RequestRevision(ctx, "e1", 0, a)
	_, _ = c.Cancel(ctx, "e1", a)
	_, _ = c.AutoRelease(ctx, "e1", 1)
	_, _ = c.RetryPayout(ctx, "e1", "bob.test.near")
	nonce := `"nonce":"` + a.Nonce + `"`
	want := []string{
		`approve_milestone {"deadline_ms":1767225600000,"escrow_id":"e1","milestone":2,` + nonce + `,"signature":"SIG"}`,
		`request_revision {"deadline_ms":1767225600000,"escrow_id":"e1","milestone":0,` + nonce + `,"signature":"SIG"}`,
		`cancel {"deadline_ms":1767225600000,"escrow_id":"e1",` + nonce + `,"signature":"SIG"}`,
		`auto_release {"escrow_id":"e1","milestone":1}`,
		`retry_payout {"account_id":"bob.test.near","escrow_id":"e1"}`,
	}
	for i, s := range sender.sent {
		fc := s.actions[0].(neartx.FunctionCall)
		if got := fc.MethodName + " " + string(fc.Args); got != want[i] {
			t.Errorf("call %d = %s, want %s", i, got, want[i])
		}
		if fc.Deposit.Sign() != 0 || fc.Gas == 0 {
			t.Errorf("call %d: deposit %v gas %d", i, fc.Deposit, fc.Gas)
		}
	}
	if fc := sender.sent[0].actions[0].(neartx.FunctionCall); fc.Gas != 300*neartx.TGas {
		t.Errorf("approve_milestone gas = %d", fc.Gas)
	}

	// The contract rejects a call: the transaction is reported, not an
	// error.
	sender.outcome = `{"transaction":{"hash":"H2"},
	  "transaction_outcome":{"id":"H2","outcome":{"logs":[],"receipt_ids":["R1"],"status":{"SuccessReceiptId":"R1"},"executor_id":"relayer.test.near"}},
	  "receipts_outcome":[{"id":"R1","outcome":{"logs":[],"receipt_ids":[],"status":{"Failure":{"ActionError":{"index":0,"kind":{"FunctionCallError":{"ExecutionError":"Smart contract panicked: milestone is not Submitted"}}}}},"executor_id":"escrow.test.near"}}]}`
	tx, err := c.ApproveMilestone(ctx, "e1", 2, a)
	if err != nil || tx.Succeeded || tx.Error != "milestone is not Submitted" || tx.Hash != "H2" {
		t.Fatalf("rejected call: %+v, %v", tx, err)
	}

	sender.err = nearrpc.ErrUnknownOutcome
	if _, err := c.AutoRelease(ctx, "e1", 0); !errors.Is(err, nearrpc.ErrUnknownOutcome) {
		t.Fatalf("unknown outcome: %v", err)
	}
}

func TestEscrowContractRelaysDelegateAction(t *testing.T) {
	key := walletsigtest.Ed25519Key(4)
	da := neartx.DelegateAction{
		SenderID: "alice.test.near", ReceiverID: "escrow.test.near", Nonce: 5, MaxBlockHeight: 100,
		PublicKey: neartx.PublicKeyOf(key),
		Actions:   []neartx.Action{neartx.FunctionCall{MethodName: "submit_milestone", Args: []byte(`{"escrow_id":"e1","milestone":0}`), Gas: neartx.TGas}},
	}
	signed := neartx.SignDelegate(da, key)
	sender := &fakeSender{account: "relayer.test.near", outcome: `{"transaction":{"hash":"MH"},
	  "transaction_outcome":{"id":"MH","outcome":{"logs":[],"receipt_ids":["D1"],"status":{"SuccessReceiptId":"D1"},"executor_id":"relayer.test.near"}},
	  "receipts_outcome":[
	    {"id":"D1","outcome":{"logs":[],"receipt_ids":["C1"],"status":{"SuccessValue":""},"executor_id":"alice.test.near"}},
	    {"id":"C1","outcome":{"logs":["EVENT_JSON:{\"standard\":\"vecrow\",\"version\":\"1.0.0\",\"event\":\"milestone_submitted\",\"data\":{\"escrow_id\":\"e1\"}}"],"receipt_ids":[],"status":{"SuccessValue":""},"executor_id":"escrow.test.near"}}]}`}
	c := repository.NewEscrowContract("escrow.test.near", &fakeViews{}, sender)

	tx, err := c.Relay(context.Background(), "e1", signed)
	if err != nil {
		t.Fatal(err)
	}
	if s := sender.sent[0]; s.receiver != "alice.test.near" || len(s.actions) != 1 {
		t.Fatalf("relayed to %s with %d actions", s.receiver, len(s.actions))
	}
	if d, ok := sender.sent[0].actions[0].(neartx.Delegate); !ok || string(d.Encode()) != string(signed.Encode()) {
		t.Fatal("the delegate action was not relayed as signed")
	}
	if !tx.Succeeded || tx.Method != "submit_milestone" || tx.SenderID != "alice.test.near" || tx.SignerID != "relayer.test.near" ||
		tx.ReceiverID != "escrow.test.near" || len(tx.Events) != 1 || tx.Events[0].ReceiptID != "C1" {
		t.Fatalf("tx = %+v", tx)
	}
}

func TestEscrowContractGetEscrow(t *testing.T) {
	views := &fakeViews{results: map[string]string{"get_escrow": "null"}}
	c := repository.NewEscrowContract("escrow.test.near", views, &fakeSender{})
	e, err := c.GetEscrow(context.Background(), "e1")
	if e != nil || err != nil || views.calls[0] != `escrow.test.near.get_escrow{"escrow_id":"e1"}` {
		t.Fatalf("GetEscrow = %v, %v (%v)", e, err, views.calls)
	}
	views.results["get_escrow"] = `{"escrow_id":"e1","freelancer":"alice.test.near","status":"Created","milestones":[],"split":[],
	  "fee_bps":50,"review_window_sec":60,"approver":{"scheme":"ed25519","address":"4zvwRjXUKGfvwnParsHAS3HuSVzV5cA4McphgmoCtajS"},
	  "refund_account":"c.test.near","total":"0","balance":"0","pending_payouts":[],"created_at_ms":5,"funded_at_ms":null}`
	e, err = c.GetEscrow(context.Background(), "e1")
	if err != nil || e.Status != domain.EscrowCreated || e.BlockHeight != 77 || e.ReviewWindowSec != 60 {
		t.Fatalf("GetEscrow = %+v, %v", e, err)
	}
}

func TestFTStorage(t *testing.T) {
	views := &fakeViews{results: map[string]string{
		"storage_balance_of":     "null",
		"storage_balance_bounds": `{"min":"1250000000000000000000","max":"1250000000000000000000"}`,
	}}
	payer := &fakeSender{account: "storage.test.near", outcome: outcome("usdc.test.near", `{"total":"1","available":"0"}`)}
	s := repository.NewFTStorage("usdc.test.near", views, payer)
	ctx := context.Background()

	if ok, err := s.IsRegistered(ctx, "bob.test.near"); ok || err != nil {
		t.Fatalf("IsRegistered(unregistered) = %v, %v", ok, err)
	}
	views.results["storage_balance_of"] = `{"total":"1250000000000000000000","available":"0"}`
	if ok, err := s.IsRegistered(ctx, "bob.test.near"); !ok || err != nil {
		t.Fatalf("IsRegistered(registered) = %v, %v", ok, err)
	}

	for range 2 {
		tx, err := s.Register(ctx, "e1", "bob.test.near")
		if err != nil || !tx.Succeeded || tx.Method != "storage_deposit" || tx.ReceiverID != "usdc.test.near" ||
			tx.SignerID != "storage.test.near" || tx.EscrowID != "e1" {
			t.Fatalf("Register = %+v, %v", tx, err)
		}
	}
	want, _ := new(big.Int).SetString("1250000000000000000000", 10)
	for _, sent := range payer.sent {
		fc := sent.actions[0].(neartx.FunctionCall)
		if sent.receiver != "usdc.test.near" || fc.MethodName != "storage_deposit" || fc.Deposit.Cmp(want) != 0 ||
			string(fc.Args) != `{"account_id":"bob.test.near","registration_only":true}` {
			t.Fatalf("storage_deposit = %s %s %v", sent.receiver, fc.Args, fc.Deposit)
		}
	}
	bounds := 0
	for _, c := range views.calls {
		if strings.Contains(c, "storage_balance_bounds") {
			bounds++
		}
	}
	if bounds != 1 {
		t.Fatalf("storage_balance_bounds read %d times", bounds)
	}
}
