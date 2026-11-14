package domain_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/harundarat/vecrow/backend/internal/domain"
)

// The get_escrow example of architecture.md §5.1.
const escrowViewExample = `{
  "escrow_id": "6f1c2a5e-8d3b-4c1e-9f00-1234567890ab",
  "freelancer": "alice.near",
  "status": "Funded",
  "milestones": [
    { "amount": "1000000000", "status": "Submitted", "submitted_at_ms": 1767225600000, "revision_count": 0 },
    { "amount": "500000000", "status": "Pending", "submitted_at_ms": null, "revision_count": 0 }
  ],
  "split": [{ "account_id": "alice.near", "bps": 7000 }, { "account_id": "bob.near", "bps": 3000 }],
  "fee_bps": 50,
  "review_window_sec": 1209600,
  "approver": { "scheme": "eip191", "address": "0x7e5f4552091a69125d5dfcb7b8c2659029395bdf" },
  "refund_account": "client.near",
  "total": "1500000000",
  "balance": "1500000000",
  "pending_payouts": [{ "account_id": "bob.near", "amount": "447750000" }],
  "created_at_ms": 1767139200000,
  "funded_at_ms": 1767142800000
}`

func TestParseEscrowView(t *testing.T) {
	c, err := domain.ParseEscrowView([]byte(escrowViewExample), 42)
	if err != nil {
		t.Fatal(err)
	}
	submitted, funded := uint64(1767225600000), uint64(1767142800000)
	want := &domain.ChainEscrow{
		EscrowID:   "6f1c2a5e-8d3b-4c1e-9f00-1234567890ab",
		Freelancer: "alice.near",
		Milestones: []domain.Milestone{
			{Amount: "1000000000", Status: domain.MilestoneSubmitted, SubmittedAtMS: &submitted},
			{Amount: "500000000", Status: domain.MilestonePending},
		},
		Split:           []domain.SplitShare{{AccountID: "alice.near", BPS: 7000}, {AccountID: "bob.near", BPS: 3000}},
		FeeBPS:          50,
		ReviewWindowSec: 1209600,
		Approver:        domain.Approver{Scheme: domain.SchemeEIP191, Address: "0x7e5f4552091a69125d5dfcb7b8c2659029395bdf"},
		RefundAccount:   "client.near",
		Status:          domain.EscrowFunded,
		Total:           "1500000000",
		Balance:         "1500000000",
		PendingPayouts:  []domain.PendingPayout{{AccountID: "bob.near", Amount: "447750000"}},
		CreatedAtMS:     1767139200000,
		FundedAtMS:      &funded,
		BlockHeight:     42,
	}
	if !reflect.DeepEqual(c, want) {
		t.Fatalf("ParseEscrowView =\n%+v\nwant\n%+v", c, want)
	}

	if c, err := domain.ParseEscrowView([]byte("null"), 1); c != nil || err != nil {
		t.Fatalf("null: %v, %v", c, err)
	}
	for _, bad := range []string{`{"status":"Open","approver":{"scheme":"eip191","address":"0x7e5f4552091a69125d5dfcb7b8c2659029395bdf"}}`, `{"status":"Created","approver":{"scheme":"x"}}`, `[`} {
		if _, err := domain.ParseEscrowView([]byte(bad), 1); err == nil {
			t.Errorf("ParseEscrowView(%s) succeeded", bad)
		}
	}
}

func TestTermsJSONRoundTrip(t *testing.T) {
	approver, err := domain.NewApprover(domain.SchemeNEP413, "client.near", "ed25519:4zvwRjXUKGfvwnParsHAS3HuSVzV5cA4McphgmoCtajS", "")
	if err != nil {
		t.Fatal(err)
	}
	terms := domain.EscrowTerms{
		EscrowID: "e1", Freelancer: "alice.near", Milestones: []string{"1", "2"},
		Split:    []domain.SplitShare{{AccountID: "alice.near", BPS: 10000}},
		Approver: approver, RefundAccount: "client.near", FeeBPS: 50, DeadlineMS: 7,
	}
	b, err := json.Marshal(terms.JSON())
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"escrow_id":"e1","freelancer":"alice.near","milestones":["1","2"],"split":[{"account_id":"alice.near","bps":10000}],` +
		`"approver":{"scheme":"nep413","account_id":"client.near","public_key":"ed25519:4zvwRjXUKGfvwnParsHAS3HuSVzV5cA4McphgmoCtajS"},` +
		`"refund_account":"client.near","fee_bps":50,"deadline_ms":7}`
	if string(b) != want {
		t.Fatalf("JSON = %s", b)
	}
	var j domain.TermsJSON
	if err := json.Unmarshal(b, &j); err != nil {
		t.Fatal(err)
	}
	back, err := j.Terms()
	if err != nil || !reflect.DeepEqual(back, terms) {
		t.Fatalf("round trip = %+v, %v", back, err)
	}
}
