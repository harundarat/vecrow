package domain_test

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/harundarat/vecrow/backend/internal/domain"
)

// The golden messages below are the examples in contracts/wallet-fixtures
// (terms_follow_the_documented_format, messages_follow_the_documented_format)
// and in architecture.md §5.3, so the backend, the fixtures the contract tests
// sign with, and the documentation agree byte for byte.

func fixtureTerms() domain.EscrowTerms {
	return domain.EscrowTerms{
		EscrowID:   "e-1",
		Freelancer: "alice.near",
		Milestones: []string{"1000000000", "500000000"},
		Split: []domain.SplitShare{
			{AccountID: "alice.near", BPS: 7000},
			{AccountID: "bob.near", BPS: 3000},
		},
		// The fixture uses a shortened address; the message builder writes
		// the label as given.
		Approver:      domain.Approver{Scheme: domain.SchemeEIP191, Address: "0xab"},
		RefundAccount: "carol.near",
		FeeBPS:        50,
		DeadlineMS:    1_700_000_000_000,
	}
}

// fixtureTermsMessage is the terms message of terms_follow_the_documented_format.
const fixtureTermsMessage = "Vecrow create_escrow\nContract: escrow.near\nEscrow: e-1\nFreelancer: alice.near\nMilestones: 1000000000,500000000\nSplit: alice.near:7000,bob.near:3000\nApprover: eip191:0xab\nRefund account: carol.near\nFee: 50 bps\nDeadline: 1700000000000"

// fixtureApproveTemplate is the approve_milestone message of
// messages_follow_the_documented_format, with the nonce as "{}" as written in
// the fixture; the nonce is 32 bytes of 0xab.
const fixtureApproveTemplate = "Vecrow approve_milestone\nContract: escrow.test.near\nEscrow: 7\nMilestone: 1\nNonce: {}\nDeadline: 1700000000000"

func TestTermsMessageGolden(t *testing.T) {
	want := fixtureTermsMessage
	if got := fixtureTerms().Message("escrow.near"); got != want {
		t.Fatalf("Message() =\n%q\nwant\n%q", got, want)
	}
}

func TestTermsNonceIsSHA256OfMessage(t *testing.T) {
	msg := fixtureTerms().Message("escrow.near")
	want := sha256.Sum256([]byte(msg))
	if got := domain.TermsNonce(msg); got != want {
		t.Fatalf("TermsNonce() = %x, want %x", got, want)
	}
}

// TestGoldenMessagesMatchWalletFixtures checks that the golden messages are
// still the ones the contract-side fixtures assert, character for character,
// so a change on either side fails a test.
func TestGoldenMessagesMatchWalletFixtures(t *testing.T) {
	path := filepath.Join("..", "..", "..", "contracts", "wallet-fixtures", "src", "lib.rs")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the wallet fixtures: %v", err)
	}
	for name, msg := range map[string]string{
		"terms_follow_the_documented_format":    fixtureTermsMessage,
		"messages_follow_the_documented_format": fixtureApproveTemplate,
	} {
		// A Go-quoted ASCII string is the same text as the Rust literal.
		if lit := strconv.Quote(msg); !strings.Contains(string(src), lit) {
			t.Errorf("%s: %s does not contain %s", name, path, lit)
		}
	}
	if !strings.Contains(string(src), "let n = nonce(0xab);") {
		t.Errorf("messages_follow_the_documented_format no longer signs with nonce(0xab)")
	}
}

func filled(b byte) [domain.NonceSize]byte {
	var n [domain.NonceSize]byte
	for i := range n {
		n[i] = b
	}
	return n
}

func TestMilestoneActionMessageGolden(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{
			name: "wallet-fixtures approve",
			got: domain.MilestoneActionMessage(domain.ActionApproveMilestone, "escrow.test.near", "7", 1,
				filled(0xab), 1_700_000_000_000),
			want: strings.Replace(fixtureApproveTemplate, "{}", strings.Repeat("ab", 32), 1),
		},
		{
			name: "architecture approve example",
			got: domain.MilestoneActionMessage(domain.ActionApproveMilestone, "escrow.test.near", "e12", 3,
				filled(0xff), 42),
			want: "Vecrow approve_milestone\nContract: escrow.test.near\nEscrow: e12\nMilestone: 3\nNonce: " +
				strings.Repeat("ff", 32) + "\nDeadline: 42",
		},
		{
			name: "request revision",
			got: domain.MilestoneActionMessage(domain.ActionRequestRevision, "escrow.test.near", "7", 0,
				filled(0x01), 5),
			want: "Vecrow request_revision\nContract: escrow.test.near\nEscrow: 7\nMilestone: 0\nNonce: " +
				strings.Repeat("01", 32) + "\nDeadline: 5",
		},
		{
			name: "cancel",
			got:  domain.CancelMessage("escrow.test.near", "7", filled(0xab), 1_700_000_000_000),
			want: "Vecrow cancel\nContract: escrow.test.near\nEscrow: 7\nNonce: " +
				strings.Repeat("ab", 32) + "\nDeadline: 1700000000000",
		},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s:\n got %q\nwant %q", tt.name, tt.got, tt.want)
		}
	}
}

func TestApproverLabel(t *testing.T) {
	// The fixture's ed25519 key for seed 1, as Phantom returns it.
	pub := ed25519.NewKeyFromSeed(make32(1)).Public().(ed25519.PublicKey)
	tests := []struct {
		scheme                        domain.WalletScheme
		accountID, publicKey, address string
		wantLabel                     string
	}{
		{scheme: domain.SchemeNEP413, accountID: "client.near", publicKey: domain.FormatNEARPublicKey(pub),
			wantLabel: "nep413:client.near"},
		{scheme: domain.SchemeEIP191, address: "0x7E5F4552091A69125d5DfCb7b8C2659029395Bdf",
			wantLabel: "eip191:0x7e5f4552091a69125d5dfcb7b8c2659029395bdf"},
		{scheme: domain.SchemeEd25519, address: domain.FormatNEARPublicKey(pub)[len("ed25519:"):],
			wantLabel: "ed25519:" + domain.FormatNEARPublicKey(pub)[len("ed25519:"):]},
	}
	for _, tt := range tests {
		a, err := domain.NewApprover(tt.scheme, tt.accountID, tt.publicKey, tt.address)
		if err != nil {
			t.Fatalf("NewApprover(%s): %v", tt.scheme, err)
		}
		if got := a.Label(); got != tt.wantLabel {
			t.Errorf("Label() = %q, want %q", got, tt.wantLabel)
		}
	}
}

func TestNewApproverRejectsMalformed(t *testing.T) {
	tests := []struct {
		name                          string
		scheme                        domain.WalletScheme
		accountID, publicKey, address string
	}{
		{name: "nep413 bad account", scheme: domain.SchemeNEP413, accountID: "A", publicKey: "ed25519:11111111111111111111111111111111"},
		{name: "nep413 bad key", scheme: domain.SchemeNEP413, accountID: "client.near", publicKey: "ed25519:abc"},
		{name: "nep413 secp key", scheme: domain.SchemeNEP413, accountID: "client.near", publicKey: "secp256k1:abc"},
		{name: "eip191 short", scheme: domain.SchemeEIP191, address: "0xab"},
		{name: "ed25519 not base58", scheme: domain.SchemeEd25519, address: "0OIl"},
		{name: "unknown scheme", scheme: "rsa", address: "x"},
	}
	for _, tt := range tests {
		if _, err := domain.NewApprover(tt.scheme, tt.accountID, tt.publicKey, tt.address); !errors.Is(err, domain.ErrInvalidArgument) {
			t.Errorf("%s: error = %v, want ErrInvalidArgument", tt.name, err)
		}
	}
}

func TestBase58LeadingZeros(t *testing.T) {
	key := make([]byte, 32)
	key[31] = 1
	s := domain.FormatNEARPublicKey(key)
	if !strings.HasPrefix(s, "ed25519:1111111111111111111111111111111") {
		t.Fatalf("FormatNEARPublicKey() = %q, want leading 1s for zero bytes", s)
	}
	back, err := domain.ParseNEARPublicKey(s)
	if err != nil || hex.EncodeToString(back) != hex.EncodeToString(key) {
		t.Fatalf("ParseNEARPublicKey(%q) = %x, %v", s, back, err)
	}
}

func make32(b byte) []byte {
	s := make([]byte, 32)
	for i := range s {
		s[i] = b
	}
	return s
}

func TestValidateMilestones(t *testing.T) {
	max := "340282366920938463463374607431768211455" // u128::MAX
	tests := []struct {
		name    string
		in      []string
		wantErr bool
	}{
		{name: "ok", in: []string{"1", "2"}},
		{name: "u128 max", in: []string{max}},
		{name: "empty", in: nil, wantErr: true},
		{name: "zero", in: []string{"0"}, wantErr: true},
		{name: "leading zero", in: []string{"01"}, wantErr: true},
		{name: "negative", in: []string{"-1"}, wantErr: true},
		{name: "decimal point", in: []string{"1.5"}, wantErr: true},
		{name: "over u128", in: []string{"340282366920938463463374607431768211456"}, wantErr: true},
		{name: "total over u128", in: []string{max, "1"}, wantErr: true},
		{name: "too many", in: strings.Split(strings.Repeat("1,", domain.MaxMilestones)+"1", ","), wantErr: true},
	}
	for _, tt := range tests {
		_, err := domain.ValidateMilestones(tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("%s: error = %v, wantErr %v", tt.name, err, tt.wantErr)
		}
	}
}

func TestValidateSplit(t *testing.T) {
	share := func(a string, b uint16) domain.SplitShare { return domain.SplitShare{AccountID: a, BPS: b} }
	many := make([]domain.SplitShare, domain.MaxSplitRecipients+1)
	for i := range many {
		many[i] = share("a"+strings.Repeat("b", i+1)+".near", 1)
	}
	tests := []struct {
		name    string
		in      []domain.SplitShare
		wantErr bool
	}{
		{name: "single", in: []domain.SplitShare{share("alice.near", 10000)}},
		{name: "two", in: []domain.SplitShare{share("alice.near", 7000), share("bob.near", 3000)}},
		{name: "empty", wantErr: true},
		{name: "under total", in: []domain.SplitShare{share("alice.near", 9999)}, wantErr: true},
		{name: "zero share", in: []domain.SplitShare{share("alice.near", 10000), share("bob.near", 0)}, wantErr: true},
		{name: "duplicate", in: []domain.SplitShare{share("alice.near", 5000), share("alice.near", 5000)}, wantErr: true},
		{name: "bad account", in: []domain.SplitShare{share("Alice", 10000)}, wantErr: true},
		{name: "too many", in: many, wantErr: true},
	}
	for _, tt := range tests {
		err := domain.ValidateSplit(tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("%s: error = %v, wantErr %v", tt.name, err, tt.wantErr)
		}
	}
}

func validTerms(t *testing.T) domain.EscrowTerms {
	t.Helper()
	approver, err := domain.NewApprover(domain.SchemeEIP191, "", "", "0x7e5f4552091a69125d5dfcb7b8c2659029395bdf")
	if err != nil {
		t.Fatal(err)
	}
	return domain.EscrowTerms{
		EscrowID:      "6f1c2a5e-8d3b-4c1e-9f00-1234567890ab",
		Freelancer:    "alice.near",
		Milestones:    []string{"1000000000", "500000000"},
		Split:         []domain.SplitShare{{AccountID: "alice.near", BPS: 7000}, {AccountID: "bob.near", BPS: 3000}},
		Approver:      approver,
		RefundAccount: "client.near",
		FeeBPS:        50,
		DeadlineMS:    1_700_000_000_000,
	}
}

func TestTermsValidate(t *testing.T) {
	const contract = "escrow.test.near"
	pub := ed25519.NewKeyFromSeed(make32(2)).Public().(ed25519.PublicKey)
	implicit := hex.EncodeToString(pub)

	if err := validTerms(t).Validate(contract); err != nil {
		t.Fatalf("valid terms: %v", err)
	}
	tests := []struct {
		name   string
		mutate func(*domain.EscrowTerms)
	}{
		{"bad escrow id", func(x *domain.EscrowTerms) { x.EscrowID = "Upper" }},
		{"long escrow id", func(x *domain.EscrowTerms) { x.EscrowID = strings.Repeat("a", 65) }},
		{"freelancer is contract", func(x *domain.EscrowTerms) { x.Freelancer = contract }},
		{"split to contract", func(x *domain.EscrowTerms) { x.Split[1].AccountID = contract }},
		{"refund to freelancer", func(x *domain.EscrowTerms) { x.RefundAccount = "alice.near" }},
		{"refund to split", func(x *domain.EscrowTerms) { x.RefundAccount = "bob.near" }},
		{"refund to contract", func(x *domain.EscrowTerms) { x.RefundAccount = contract }},
		{"nep413 approver is freelancer", func(x *domain.EscrowTerms) {
			x.Approver = domain.Approver{Scheme: domain.SchemeNEP413, AccountID: "alice.near", PublicKey: domain.FormatNEARPublicKey(pub)}
		}},
		{"nep413 approver key is split implicit", func(x *domain.EscrowTerms) {
			x.Approver = domain.Approver{Scheme: domain.SchemeNEP413, AccountID: "client.near", PublicKey: domain.FormatNEARPublicKey(pub)}
			x.Split[1].AccountID = implicit
		}},
		{"eth-implicit approver is split", func(x *domain.EscrowTerms) {
			x.Split[1].AccountID = x.Approver.Address
		}},
		{"solana approver is implicit freelancer", func(x *domain.EscrowTerms) {
			x.Approver = domain.Approver{Scheme: domain.SchemeEd25519, Address: domain.FormatNEARPublicKey(pub)[len("ed25519:"):]}
			x.Freelancer = implicit
			x.Split[0].AccountID = implicit
		}},
		{"no approver", func(x *domain.EscrowTerms) { x.Approver = domain.Approver{} }},
		{"bad split", func(x *domain.EscrowTerms) { x.Split[1].BPS = 2999 }},
		{"bad milestones", func(x *domain.EscrowTerms) { x.Milestones = nil }},
	}
	for _, tt := range tests {
		terms := validTerms(t)
		tt.mutate(&terms)
		if err := terms.Validate(contract); !errors.Is(err, domain.ErrInvalidArgument) {
			t.Errorf("%s: error = %v, want ErrInvalidArgument", tt.name, err)
		}
	}
}

func TestCheckFreelancerKey(t *testing.T) {
	pub := ed25519.NewKeyFromSeed(make32(3)).Public().(ed25519.PublicKey)
	other := ed25519.NewKeyFromSeed(make32(4)).Public().(ed25519.PublicKey)

	terms := validTerms(t)
	if err := terms.CheckFreelancerKey(pub); err != nil {
		t.Fatalf("named freelancer: %v", err)
	}

	terms.Freelancer = hex.EncodeToString(pub)
	terms.Split = []domain.SplitShare{{AccountID: terms.Freelancer, BPS: 10000}}
	if err := terms.CheckFreelancerKey(pub); err != nil {
		t.Fatalf("implicit freelancer with own key: %v", err)
	}
	if err := terms.CheckFreelancerKey(other); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("implicit freelancer with another key: error = %v", err)
	}

	terms = validTerms(t)
	terms.Approver = domain.Approver{Scheme: domain.SchemeNEP413, AccountID: "client.near", PublicKey: domain.FormatNEARPublicKey(pub)}
	if err := terms.CheckFreelancerKey(pub); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("approver holds the freelancer key: error = %v", err)
	}
}

func TestParseRole(t *testing.T) {
	if r, err := domain.ParseRole("client"); err != nil || r.Other() != domain.RoleFreelancer {
		t.Fatalf("ParseRole(client) = %q, %v", r, err)
	}
	if _, err := domain.ParseRole("member"); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("ParseRole(member) error = %v", err)
	}
}
