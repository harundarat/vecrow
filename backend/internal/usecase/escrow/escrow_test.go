package escrow_test

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/harundarat/vecrow/backend/internal/domain"
	"github.com/harundarat/vecrow/backend/internal/neartx"
	"github.com/harundarat/vecrow/backend/internal/usecase/escrow"
	"github.com/harundarat/vecrow/backend/internal/walletsig/walletsigtest"
)

const (
	contractID = "escrow.test.near"
	freelancer = "alice.test.near"
	member     = "bob.test.near"
	refundAcct = "client.test.near"
	accF       = "account-freelancer"
	accC       = "account-client"
)

var (
	freelancerKey = walletsigtest.Ed25519Key(1)
	clientEVM     = walletsigtest.NewEVMKey(1)
	start         = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
)

type env struct {
	svc      *escrow.Service
	repo     *fakeRepo
	contract *fakeContract
	storage  *fakeStorage
	guests   *fakeGuests
	keys     fakeKeys
	now      time.Time
}

func newEnv(t *testing.T) *env {
	t.Helper()
	e := &env{
		repo:     newFakeRepo(),
		contract: newFakeContract(contractID),
		storage:  &fakeStorage{registered: map[string]bool{}},
		guests:   &fakeGuests{},
		keys:     fakeKeys{full: map[string]bool{freelancer + "|" + walletsigtest.NEARPublicKey(freelancerKey): true}},
		now:      start,
	}
	wallets := fakeWallets{owned: map[string][]domain.Wallet{
		accF: {{Scheme: domain.SchemeNEP413, Address: freelancer}},
		accC: {{Scheme: domain.SchemeEIP191, Address: clientEVM.Address()}},
	}}
	e.svc = escrow.NewService(escrow.Deps{
		Repo: e.repo, Tx: fakeTx{}, Wallets: wallets, Keys: e.keys, Contract: e.contract,
		Storage: e.storage, Guests: e.guests,
	}, escrow.Config{
		FeeBPS: 50, TermsTTL: time.Hour, AppURL: "https://app.example/", RelayerMaxGas: 100 * neartx.TGas,
		ExpiryGrace: time.Minute,
	}, escrow.WithClock(func() time.Time { return e.now }))
	return e
}

func freelancerDetails() escrow.DetailsInput {
	return escrow.DetailsInput{Freelancer: &escrow.FreelancerInput{Account: freelancer, Split: []domain.SplitShare{
		{AccountID: freelancer, BPS: 7000}, {AccountID: member, BPS: 3000},
	}}}
}

func clientDetails() escrow.DetailsInput {
	return escrow.DetailsInput{Client: &escrow.ClientInput{
		Approver:      escrow.ApproverInput{Scheme: "eip191", Address: clientEVM.Address()},
		RefundAccount: refundAcct,
	}}
}

func isKind(err, kind error) bool { return errors.Is(err, kind) }

func (e *env) create(t *testing.T, role string) domain.Escrow {
	t.Helper()
	in := escrow.CreateInput{Role: role, Milestones: []string{"1000000", "500000"}}
	if role == "freelancer" {
		in.AccountID, in.Details = accF, freelancerDetails()
	} else {
		in.AccountID, in.Details = accC, clientDetails()
	}
	esc, err := e.svc.Create(context.Background(), in)
	if err != nil {
		t.Fatalf("Create(%s): %v", role, err)
	}
	return esc
}

func signFreelancer(terms escrow.Terms) (sig, pub string) {
	return walletsigtest.SignNEP413(freelancerKey, terms.Message, domain.TermsNonce(terms.Message), contractID),
		walletsigtest.NEARPublicKey(freelancerKey)
}

// join submits the other party's details and signature.
func (e *env) join(t *testing.T, esc domain.Escrow) escrow.SubmitResult {
	t.Helper()
	ctx := context.Background()
	details := clientDetails()
	if esc.CreatorRole == domain.RoleClient {
		details = freelancerDetails()
	}
	terms, err := e.svc.BuildTerms(ctx, esc.ID, details)
	if err != nil {
		t.Fatalf("BuildTerms: %v", err)
	}
	in := escrow.SubmitInput{Details: details, DeadlineMS: terms.Terms.DeadlineMS}
	if esc.CreatorRole == domain.RoleClient {
		in.Signature, in.PublicKey = signFreelancer(terms)
	} else {
		in.Signature = clientEVM.SignEIP191(terms.Message)
	}
	res, err := e.svc.SubmitCounterparty(ctx, esc.ID, in)
	if err != nil {
		t.Fatalf("SubmitCounterparty: %v", err)
	}
	return res
}

// approve signs the submitted terms as the creator.
func (e *env) approve(t *testing.T, esc domain.Escrow) domain.Escrow {
	t.Helper()
	got := e.repo.escrow(esc.ID)
	msg := got.Submission.Message
	var in escrow.ApproveInput
	account := accC
	if esc.CreatorRole == domain.RoleFreelancer {
		account = accF
		in.Signature = walletsigtest.SignNEP413(freelancerKey, msg, domain.TermsNonce(msg), contractID)
		in.PublicKey = walletsigtest.NEARPublicKey(freelancerKey)
	} else {
		in.Signature = clientEVM.SignEIP191(msg)
	}
	out, err := e.svc.Approve(context.Background(), account, esc.ID, in)
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	return out
}

// onChain takes a new escrow through creation on-chain.
func (e *env) onChain(t *testing.T, role string) domain.Escrow {
	t.Helper()
	esc := e.create(t, role)
	e.join(t, esc)
	e.approve(t, esc)
	if err := e.svc.CreateOnChain(context.Background(), esc.ID); err != nil {
		t.Fatalf("CreateOnChain: %v", err)
	}
	got := e.repo.escrow(esc.ID)
	if got.Status != domain.EscrowCreated {
		t.Fatalf("status after creation = %s", got.Status)
	}
	return got
}

func TestCreateEscrowIsOpenWithLink(t *testing.T) {
	for _, role := range []string{"freelancer", "client"} {
		e := newEnv(t)
		esc := e.create(t, role)
		if esc.Status != domain.EscrowOpen || esc.FeeBPS != 50 || len(esc.Milestones) != 2 {
			t.Fatalf("%s: escrow = %+v", role, esc)
		}
		if domain.ValidateEscrowID(esc.ID) != nil || len(esc.ID) != 36 {
			t.Fatalf("%s: escrow ID %q is not a lowercase UUID", role, esc.ID)
		}
		if link := e.svc.Link(esc.ID); link != "https://app.example/escrows/"+esc.ID {
			t.Fatalf("link = %s", link)
		}
		if len(e.repo.parties[esc.ID]) == 0 {
			t.Fatalf("%s: no parties stored", role)
		}
	}
	if len(newEnv(t).contract.calls) != 0 {
		t.Fatal("creating an escrow called the contract")
	}
}

func TestCreateRejectsAddressNotOwned(t *testing.T) {
	e := newEnv(t)
	_, err := e.svc.Create(context.Background(), escrow.CreateInput{
		AccountID: accC, Role: "freelancer", Milestones: []string{"1"}, Details: freelancerDetails(),
	})
	if !isKind(err, domain.ErrForbidden) {
		t.Fatalf("freelancer not owned: error = %v", err)
	}
	_, err = e.svc.Create(context.Background(), escrow.CreateInput{
		AccountID: accF, Role: "client", Milestones: []string{"1"}, Details: clientDetails(),
	})
	if !isKind(err, domain.ErrForbidden) {
		t.Fatalf("approver not owned: error = %v", err)
	}
}

func TestCreateValidatesInput(t *testing.T) {
	e := newEnv(t)
	bad := freelancerDetails()
	bad.Freelancer.Split[1].BPS = 2000
	tests := map[string]escrow.CreateInput{
		"role":            {AccountID: accF, Role: "member", Milestones: []string{"1"}, Details: freelancerDetails()},
		"other details":   {AccountID: accF, Role: "freelancer", Milestones: []string{"1"}, Details: clientDetails()},
		"both details":    {AccountID: accF, Role: "freelancer", Milestones: []string{"1"}, Details: escrow.DetailsInput{Freelancer: freelancerDetails().Freelancer, Client: clientDetails().Client}},
		"no milestones":   {AccountID: accF, Role: "freelancer", Details: freelancerDetails()},
		"zero milestone":  {AccountID: accF, Role: "freelancer", Milestones: []string{"0"}, Details: freelancerDetails()},
		"split total":     {AccountID: accF, Role: "freelancer", Milestones: []string{"1"}, Details: bad},
		"bad approver":    {AccountID: accC, Role: "client", Milestones: []string{"1"}, Details: escrow.DetailsInput{Client: &escrow.ClientInput{Approver: escrow.ApproverInput{Scheme: "eip191", Address: "0x1"}, RefundAccount: refundAcct}}},
		"refund contract": {AccountID: accC, Role: "client", Milestones: []string{"1"}, Details: escrow.DetailsInput{Client: &escrow.ClientInput{Approver: escrow.ApproverInput{Scheme: "eip191", Address: clientEVM.Address()}, RefundAccount: contractID}}},
	}
	for name, in := range tests {
		if _, err := e.svc.Create(context.Background(), in); !isKind(err, domain.ErrInvalidArgument) {
			t.Errorf("%s: error = %v, want invalid argument", name, err)
		}
	}
}

func TestBothDirectionsReachCreated(t *testing.T) {
	for _, role := range []string{"freelancer", "client"} {
		t.Run(role, func(t *testing.T) {
			e := newEnv(t)
			esc := e.create(t, role)
			res := e.join(t, esc)
			if res.GuestToken == "" || res.Escrow.Status != domain.EscrowPendingApproval {
				t.Fatalf("submission result = %+v", res)
			}
			wantWallet := domain.Wallet{Scheme: domain.SchemeEIP191, Address: clientEVM.Address()}
			if role == "client" {
				wantWallet = domain.Wallet{Scheme: domain.SchemeNEP413, Address: freelancer}
			}
			if g := e.guests.issued[0]; g.EscrowID != esc.ID || g.Wallet != wantWallet {
				t.Fatalf("guest token for %+v, want %s %+v", g, esc.ID, wantWallet)
			}
			if len(e.contract.calls) != 0 {
				t.Fatal("the contract was called before approval")
			}
			approved := e.approve(t, esc)
			if approved.Status != domain.EscrowApproved || len(e.contract.calls) != 0 {
				t.Fatalf("after approval: status %s, calls %v", approved.Status, e.contract.calls)
			}

			if err := e.svc.CreateOnChain(context.Background(), esc.ID); err != nil {
				t.Fatalf("CreateOnChain: %v", err)
			}
			got := e.repo.escrow(esc.ID)
			if got.Status != domain.EscrowCreated || got.Submission.Status != domain.SubmissionCreated {
				t.Fatalf("after creation: %s / %s", got.Status, got.Submission.Status)
			}
			// Storage was registered for every split recipient and the refund
			// account before create_escrow.
			if strings.Join(e.storage.registers, ",") != freelancer+","+member+","+refundAcct {
				t.Fatalf("registered = %v", e.storage.registers)
			}
			if strings.Join(e.contract.calls, ",") != "create_escrow" {
				t.Fatalf("contract calls = %v", e.contract.calls)
			}
			chain := e.contract.escrows[esc.ID]
			terms, _ := got.Terms()
			if !chain.MatchesTerms(terms) {
				t.Fatal("contract escrow does not have the signed terms")
			}
			txs := e.repo.txs[esc.ID]
			if len(txs) != 4 || txs[3].Method != "create_escrow" || len(txs[3].Events) != 1 {
				t.Fatalf("recorded transactions = %+v", txs)
			}

			// Running the job again changes nothing.
			if err := e.svc.CreateOnChain(context.Background(), esc.ID); err != nil || len(e.contract.calls) != 1 {
				t.Fatalf("second run: %v, calls %v", err, e.contract.calls)
			}
		})
	}
}

func TestSubmissionRules(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	esc := e.create(t, "freelancer")
	terms, err := e.svc.BuildTerms(ctx, esc.ID, clientDetails())
	if err != nil {
		t.Fatal(err)
	}
	if terms.Recipient != contractID || terms.Terms.DeadlineMS != uint64(start.Add(time.Hour).UnixMilli()) {
		t.Fatalf("terms = %+v", terms)
	}
	nonce := domain.TermsNonce(terms.Message)
	if terms.Nonce != hex.EncodeToString(nonce[:]) {
		t.Fatal("terms nonce is not sha256 of the message")
	}

	valid := escrow.SubmitInput{Details: clientDetails(), DeadlineMS: terms.Terms.DeadlineMS,
		Signature: clientEVM.SignEIP191(terms.Message)}

	other := walletsigtest.NewEVMKey(2)
	wrongSigner := valid
	wrongSigner.Signature = other.SignEIP191(terms.Message)
	if _, err := e.svc.SubmitCounterparty(ctx, esc.ID, wrongSigner); !isKind(err, domain.ErrUnauthorized) {
		t.Errorf("wrong signer: %v", err)
	}
	// A signature over other terms (here: another deadline) does not verify.
	shifted := valid
	shifted.DeadlineMS--
	if _, err := e.svc.SubmitCounterparty(ctx, esc.ID, shifted); !isKind(err, domain.ErrUnauthorized) {
		t.Errorf("other terms: %v", err)
	}
	late := valid
	late.DeadlineMS = uint64(start.Add(2 * time.Hour).UnixMilli())
	if _, err := e.svc.SubmitCounterparty(ctx, esc.ID, late); !isKind(err, domain.ErrInvalidArgument) {
		t.Errorf("deadline too late: %v", err)
	}
	// Refunds to a split recipient break R-04.
	breaks := valid
	breaks.Details = clientDetails()
	breaks.Details.Client.RefundAccount = member
	if _, err := e.svc.SubmitCounterparty(ctx, esc.ID, breaks); !isKind(err, domain.ErrInvalidArgument) {
		t.Errorf("refund to split member: %v", err)
	}
	if e.repo.escrow(esc.ID).Status != domain.EscrowOpen {
		t.Fatal("a rejected submission changed the escrow")
	}

	if _, err := e.svc.SubmitCounterparty(ctx, esc.ID, valid); err != nil {
		t.Fatalf("valid submission: %v", err)
	}
	if _, err := e.svc.SubmitCounterparty(ctx, esc.ID, valid); !isKind(err, domain.ErrConflict) {
		t.Errorf("second submission: %v", err)
	}
	if _, err := e.svc.BuildTerms(ctx, esc.ID, clientDetails()); !isKind(err, domain.ErrConflict) {
		t.Errorf("terms while pending: %v", err)
	}
}

func TestFreelancerKeyMustBeFullAccess(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	esc := e.create(t, "client")
	terms, _ := e.svc.BuildTerms(ctx, esc.ID, freelancerDetails())
	stranger := walletsigtest.Ed25519Key(9)
	in := escrow.SubmitInput{Details: freelancerDetails(), DeadlineMS: terms.Terms.DeadlineMS,
		Signature: walletsigtest.SignNEP413(stranger, terms.Message, domain.TermsNonce(terms.Message), contractID),
		PublicKey: walletsigtest.NEARPublicKey(stranger)}
	if _, err := e.svc.SubmitCounterparty(ctx, esc.ID, in); !isKind(err, domain.ErrUnauthorized) {
		t.Fatalf("key of another account: %v", err)
	}
	// A NEP-413 signature made for another recipient does not verify.
	in.Signature = walletsigtest.SignNEP413(freelancerKey, terms.Message, domain.TermsNonce(terms.Message), "other.near")
	in.PublicKey = walletsigtest.NEARPublicKey(freelancerKey)
	if _, err := e.svc.SubmitCounterparty(ctx, esc.ID, in); !isKind(err, domain.ErrUnauthorized) {
		t.Fatalf("other recipient: %v", err)
	}
}

func TestOnlyCreatorApprovesWithSignature(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	esc := e.create(t, "freelancer")
	e.join(t, esc)
	msg := e.repo.escrow(esc.ID).Submission.Message
	sig := walletsigtest.SignNEP413(freelancerKey, msg, domain.TermsNonce(msg), contractID)
	pub := walletsigtest.NEARPublicKey(freelancerKey)

	if _, err := e.svc.Approve(ctx, accC, esc.ID, escrow.ApproveInput{Signature: sig, PublicKey: pub}); !isKind(err, domain.ErrForbidden) {
		t.Errorf("not the creator: %v", err)
	}
	if _, err := e.svc.Reject(ctx, accC, esc.ID); !isKind(err, domain.ErrForbidden) {
		t.Errorf("reject by not the creator: %v", err)
	}
	otherMsg := strings.Replace(msg, "Fee: 50", "Fee: 51", 1)
	bad := walletsigtest.SignNEP413(freelancerKey, otherMsg, domain.TermsNonce(otherMsg), contractID)
	if _, err := e.svc.Approve(ctx, accF, esc.ID, escrow.ApproveInput{Signature: bad, PublicKey: pub}); !isKind(err, domain.ErrUnauthorized) {
		t.Errorf("signature over other terms: %v", err)
	}
	if got := e.repo.escrow(esc.ID); got.Status != domain.EscrowPendingApproval {
		t.Fatalf("status = %s", got.Status)
	}
	if _, err := e.svc.Approve(ctx, accF, esc.ID, escrow.ApproveInput{Signature: sig, PublicKey: pub}); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if _, err := e.svc.Approve(ctx, accF, esc.ID, escrow.ApproveInput{Signature: sig, PublicKey: pub}); !isKind(err, domain.ErrConflict) {
		t.Errorf("approve twice: %v", err)
	}
}

func TestRejectAndExpiryReopen(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	esc := e.create(t, "client")
	e.join(t, esc)
	got, err := e.svc.Reject(ctx, accC, esc.ID)
	if err != nil || got.Status != domain.EscrowOpen || got.Submission != nil {
		t.Fatalf("reject: %+v, %v", got, err)
	}
	if s := e.repo.submissions[esc.ID][0]; s.Status != domain.SubmissionRejected {
		t.Fatalf("submission status = %s", s.Status)
	}
	// The link works again.
	e.join(t, esc)

	// The deadline passes before the creator approves.
	e.now = start.Add(2 * time.Hour)
	if _, err := e.svc.FindDue(ctx, time.Minute, 10); err != nil {
		t.Fatal(err)
	}
	if got := e.repo.escrow(esc.ID); got.Status != domain.EscrowOpen || got.Submission != nil {
		t.Fatalf("after expiry: %s", got.Status)
	}
	if s := e.repo.submissions[esc.ID][1]; s.Status != domain.SubmissionExpired {
		t.Fatalf("submission status = %s", s.Status)
	}

	// Approving an expired submission is refused and reopens the escrow.
	e.now = start.Add(3 * time.Hour)
	e.join(t, esc)
	e.now = e.now.Add(2 * time.Hour)
	msg := e.repo.escrow(esc.ID).Submission.Message
	_, err = e.svc.Approve(ctx, accC, esc.ID, escrow.ApproveInput{Signature: clientEVM.SignEIP191(msg)})
	if !isKind(err, domain.ErrConflict) || e.repo.escrow(esc.ID).Status != domain.EscrowOpen {
		t.Fatalf("approve after deadline: %v", err)
	}
}

func TestCreateOnChainRejectedReturnsToOpen(t *testing.T) {
	e := newEnv(t)
	esc := e.create(t, "freelancer")
	e.join(t, esc)
	e.approve(t, esc)
	e.contract.reject = "fee_bps does not match the configuration"
	if err := e.svc.CreateOnChain(context.Background(), esc.ID); err != nil {
		t.Fatal(err)
	}
	got := e.repo.escrow(esc.ID)
	if got.Status != domain.EscrowOpen || got.Submission != nil {
		t.Fatalf("status = %s", got.Status)
	}
	if s := e.repo.submissions[esc.ID][0]; s.Status != domain.SubmissionFailed || !strings.Contains(s.FailureReason, "fee_bps") {
		t.Fatalf("submission = %+v", s)
	}
}

func TestCreateOnChainAfterCrashMarksCreated(t *testing.T) {
	e := newEnv(t)
	esc := e.create(t, "freelancer")
	e.join(t, esc)
	e.approve(t, esc)
	// A previous run sent create_escrow but stopped before recording it.
	got := e.repo.escrow(esc.ID)
	terms, _ := got.Terms()
	f, c := got.Submission.Signatures(got.CreatorRole)
	if _, err := e.contract.CreateEscrow(context.Background(), terms, f, c); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.CreateOnChain(context.Background(), esc.ID); err != nil {
		t.Fatal(err)
	}
	if got := e.repo.escrow(esc.ID); got.Status != domain.EscrowCreated {
		t.Fatalf("status = %s", got.Status)
	}
	if len(e.contract.calls) != 1 {
		t.Fatalf("create_escrow sent again: %v", e.contract.calls)
	}
}

func TestCreateOnChainAfterDeadline(t *testing.T) {
	e := newEnv(t)
	esc := e.create(t, "freelancer")
	e.join(t, esc)
	e.approve(t, esc)
	e.now = start.Add(time.Hour + time.Second)
	if err := e.svc.CreateOnChain(context.Background(), esc.ID); !errors.Is(err, escrow.ErrNotDue) {
		t.Fatalf("within grace: %v", err)
	}
	e.now = start.Add(time.Hour + 2*time.Minute)
	if err := e.svc.CreateOnChain(context.Background(), esc.ID); err != nil {
		t.Fatal(err)
	}
	if got := e.repo.escrow(esc.ID); got.Status != domain.EscrowOpen || len(e.contract.calls) != 0 {
		t.Fatalf("status %s, calls %v", got.Status, e.contract.calls)
	}
}

func delegateFor(sender string, key ed25519.PrivateKey, receiver string, actions ...neartx.Action) []byte {
	da := neartx.DelegateAction{SenderID: sender, ReceiverID: receiver, Actions: actions, Nonce: 1,
		MaxBlockHeight: 100, PublicKey: neartx.PublicKeyOf(key)}
	return neartx.SignDelegate(da, key).Encode()
}

func submitCall(escrowID string, milestone int) neartx.FunctionCall {
	return neartx.FunctionCall{MethodName: "submit_milestone",
		Args: []byte(`{"escrow_id":"` + escrowID + `","milestone":` + itoa(milestone) + `}`), Gas: 30 * neartx.TGas}
}

func itoa(i int) string { return big.NewInt(int64(i)).String() }

func TestRelayerPolicy(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	esc := e.onChain(t, "freelancer")
	calls := len(e.contract.calls)

	ok := submitCall(esc.ID, 0)
	withDeposit := ok
	withDeposit.Deposit = big.NewInt(1)
	tooMuchGas := ok
	tooMuchGas.Gas = 101 * neartx.TGas
	other := walletsigtest.Ed25519Key(7)

	signed := delegateFor(freelancer, freelancerKey, contractID, ok)
	tampered := append([]byte(nil), signed...)
	tampered[len(tampered)-1] ^= 1

	tests := []struct {
		name string
		raw  []byte
		n    uint32
		kind error
	}{
		{"other contract", delegateFor(freelancer, freelancerKey, "usdc.test.near", ok), 0, domain.ErrForbidden},
		{"method not allowed", delegateFor(freelancer, freelancerKey, contractID, neartx.FunctionCall{MethodName: "approve_milestone", Args: []byte(`{}`), Gas: neartx.TGas}), 0, domain.ErrForbidden},
		{"wrong method for endpoint", delegateFor(freelancer, freelancerKey, contractID, neartx.FunctionCall{MethodName: "cancel", Args: []byte(`{"escrow_id":"` + esc.ID + `"}`), Gas: neartx.TGas}), 0, domain.ErrForbidden},
		{"transfer", delegateFor(freelancer, freelancerKey, contractID, neartx.Transfer{Deposit: big.NewInt(1)}), 0, domain.ErrForbidden},
		{"two calls", delegateFor(freelancer, freelancerKey, contractID, ok, ok), 0, domain.ErrForbidden},
		{"deposit", delegateFor(freelancer, freelancerKey, contractID, withDeposit), 0, domain.ErrForbidden},
		{"gas above cap", delegateFor(freelancer, freelancerKey, contractID, tooMuchGas), 0, domain.ErrForbidden},
		{"not the freelancer", delegateFor("mallory.test.near", other, contractID, ok), 0, domain.ErrForbidden},
		{"other escrow", delegateFor(freelancer, freelancerKey, contractID, submitCall("other", 0)), 0, domain.ErrInvalidArgument},
		{"other milestone", delegateFor(freelancer, freelancerKey, contractID, submitCall(esc.ID, 1)), 0, domain.ErrInvalidArgument},
		{"extra argument", delegateFor(freelancer, freelancerKey, contractID, neartx.FunctionCall{MethodName: "submit_milestone", Args: []byte(`{"escrow_id":"` + esc.ID + `","milestone":0,"x":1}`), Gas: neartx.TGas}), 0, domain.ErrInvalidArgument},
		{"bad signature", tampered, 0, domain.ErrUnauthorized},
		{"malformed", []byte{1, 2, 3}, 0, domain.ErrInvalidArgument},
	}
	for _, tt := range tests {
		if _, err := e.svc.SubmitMilestone(ctx, esc.ID, tt.n, tt.raw); !isKind(err, tt.kind) {
			t.Errorf("%s: error = %v, want %v", tt.name, err, tt.kind)
		}
	}
	if len(e.contract.calls) != calls {
		t.Fatalf("rejected delegate actions reached the contract: %v", e.contract.calls[calls:])
	}

	res, err := e.svc.SubmitMilestone(ctx, esc.ID, 0, signed)
	if err != nil {
		t.Fatalf("valid submit: %v", err)
	}
	if res.Transaction.SenderID != freelancer || len(e.contract.relayed) != 1 {
		t.Fatalf("relayed = %+v", res.Transaction)
	}
	if res.Escrow.Milestones[0].Status != domain.MilestoneSubmitted {
		t.Fatalf("milestone after submit = %+v", res.Escrow.Milestones[0])
	}
}

func approverAction(msg func(n [32]byte) string, deadline uint64) escrow.ApproverAction {
	var n [32]byte
	n[0], n[31] = 0xab, 0xcd
	return escrow.ApproverAction{Nonce: hex.EncodeToString(n[:]), DeadlineMS: deadline, Signature: clientEVM.SignEIP191(msg(n))}
}

func TestApproveAndRevisionAreVerifiedLocally(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	esc := e.onChain(t, "client")
	e.contract.setMilestone(esc.ID, 0, domain.Milestone{Amount: "1000000", Status: domain.MilestoneSubmitted})
	deadline := uint64(start.Add(time.Minute).UnixMilli())
	approveMsg := func(m uint32) func(n [32]byte) string {
		return func(n [32]byte) string {
			return domain.MilestoneActionMessage(domain.ActionApproveMilestone, contractID, esc.ID, m, n, deadline)
		}
	}
	calls := len(e.contract.calls)

	wrongMilestone := approverAction(approveMsg(1), deadline)
	if _, err := e.svc.ApproveMilestone(ctx, esc.ID, 0, wrongMilestone); !isKind(err, domain.ErrUnauthorized) {
		t.Errorf("signature for another milestone: %v", err)
	}
	expired := approverAction(approveMsg(0), uint64(start.UnixMilli()))
	if _, err := e.svc.ApproveMilestone(ctx, esc.ID, 0, expired); !isKind(err, domain.ErrInvalidArgument) {
		t.Errorf("expired deadline: %v", err)
	}
	upper := approverAction(approveMsg(0), deadline)
	upper.Nonce = strings.ToUpper(upper.Nonce)
	if _, err := e.svc.ApproveMilestone(ctx, esc.ID, 0, upper); !isKind(err, domain.ErrInvalidArgument) {
		t.Errorf("uppercase nonce: %v", err)
	}
	if _, err := e.svc.ApproveMilestone(ctx, esc.ID, 5, approverAction(approveMsg(5), deadline)); !isKind(err, domain.ErrNotFound) {
		t.Errorf("unknown milestone: %v", err)
	}
	if len(e.contract.calls) != calls {
		t.Fatalf("invalid approvals reached the contract")
	}

	revisionMsg := func(n [32]byte) string {
		return domain.MilestoneActionMessage(domain.ActionRequestRevision, contractID, esc.ID, 0, n, deadline)
	}
	res, err := e.svc.RequestRevision(ctx, esc.ID, 0, approverAction(revisionMsg, deadline), "  fix the logo  ")
	if err != nil {
		t.Fatalf("revision: %v", err)
	}
	if res.Escrow.Milestones[0].Status != domain.MilestoneRevisionRequested {
		t.Fatalf("milestone = %+v", res.Escrow.Milestones[0])
	}
	if n := e.repo.notes[esc.ID]; len(n) != 1 || n[0].Reason != "fix the logo" || n[0].TxHash != res.Transaction.Hash {
		t.Fatalf("notes = %+v", n)
	}

	// The contract rejects the approval: a conflict, with the failed
	// transaction recorded.
	e.contract.reject = "milestone is not Submitted"
	_, err = e.svc.ApproveMilestone(ctx, esc.ID, 0, approverAction(approveMsg(0), deadline))
	if !isKind(err, domain.ErrConflict) || !strings.Contains(err.Error(), "milestone is not Submitted") {
		t.Fatalf("rejected approval: %v", err)
	}
	txs := e.repo.txs[esc.ID]
	if last := txs[len(txs)-1]; last.Succeeded || last.Method != "approve_milestone" {
		t.Fatalf("last transaction = %+v", last)
	}

	// An unknown outcome is reported as unavailable.
	e.contract.unavailable = true
	_, err = e.svc.ApproveMilestone(ctx, esc.ID, 0, approverAction(approveMsg(0), deadline))
	if !isKind(err, domain.ErrUnavailable) {
		t.Fatalf("unknown outcome: %v", err)
	}
}

func TestCancel(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)

	esc := e.create(t, "freelancer")
	if _, err := e.svc.Cancel(ctx, esc.ID, escrow.CancelInput{AccountID: accC}); !isKind(err, domain.ErrForbidden) {
		t.Errorf("cancel by another account: %v", err)
	}
	if _, err := e.svc.Cancel(ctx, esc.ID, escrow.CancelInput{}); !isKind(err, domain.ErrUnauthorized) {
		t.Errorf("cancel without session: %v", err)
	}
	res, err := e.svc.Cancel(ctx, esc.ID, escrow.CancelInput{AccountID: accF})
	if err != nil || res.Escrow.Status != domain.EscrowCancelled || len(e.contract.calls) != 0 {
		t.Fatalf("creator cancel: %+v, %v", res.Escrow.Status, err)
	}

	approved := e.create(t, "freelancer")
	e.join(t, approved)
	e.approve(t, approved)
	if _, err := e.svc.Cancel(ctx, approved.ID, escrow.CancelInput{AccountID: accF}); !isKind(err, domain.ErrConflict) {
		t.Errorf("cancel while Approved: %v", err)
	}

	// On-chain: by the approver's signature, then by the freelancer's
	// delegate action.
	onchain := e.onChain(t, "client")
	deadline := uint64(start.Add(time.Minute).UnixMilli())
	a := approverAction(func(n [32]byte) string { return domain.CancelMessage(contractID, onchain.ID, n, deadline) }, deadline)
	res, err = e.svc.Cancel(ctx, onchain.ID, escrow.CancelInput{Approver: &a})
	if err != nil || res.Escrow.Status != domain.EscrowCancelled {
		t.Fatalf("approver cancel: %v", err)
	}
	if e.contract.lastAction != a {
		t.Fatal("the signed cancellation was not forwarded as given")
	}

	byFreelancer := e.onChain(t, "freelancer")
	cancel := neartx.FunctionCall{MethodName: "cancel", Args: []byte(`{"escrow_id":"` + byFreelancer.ID + `"}`), Gas: 20 * neartx.TGas}
	res, err = e.svc.Cancel(ctx, byFreelancer.ID, escrow.CancelInput{Delegate: delegateFor(freelancer, freelancerKey, contractID, cancel)})
	if err != nil || res.Escrow.Status != domain.EscrowCancelled {
		t.Fatalf("freelancer cancel: %v", err)
	}
}

func TestAutoReleaseAndRetryPayoutJobs(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	esc := e.onChain(t, "freelancer")
	chain := e.contract.escrows[esc.ID]
	chain.Status = domain.EscrowFunded
	submitted := uint64(start.UnixMilli())
	e.contract.setMilestone(esc.ID, 0, domain.Milestone{Amount: "1000000", Status: domain.MilestoneSubmitted, SubmittedAtMS: &submitted})
	if err := e.svc.Sync(ctx, esc.ID); err != nil {
		t.Fatal(err)
	}

	// The review window (60 s) has not ended.
	due, err := e.svc.FindDue(ctx, time.Minute, 10)
	if err != nil || len(due.AutoRelease) != 0 {
		t.Fatalf("due early: %+v, %v", due, err)
	}
	if err := e.svc.AutoRelease(ctx, esc.ID, 0); err != nil || strings.Contains(strings.Join(e.contract.calls, ","), "auto_release") {
		t.Fatalf("auto_release before the window: %v %v", err, e.contract.calls)
	}

	e.now = start.Add(61 * time.Second)
	due, _ = e.svc.FindDue(ctx, time.Minute, 10)
	if len(due.AutoRelease) != 1 || due.AutoRelease[0] != (escrow.MilestoneRef{EscrowID: esc.ID, Milestone: 0}) {
		t.Fatalf("due = %+v", due)
	}
	if err := e.svc.AutoRelease(ctx, esc.ID, 0); err != nil {
		t.Fatal(err)
	}
	if got := e.repo.escrow(esc.ID); got.Milestones[0].Status != domain.MilestoneReleased {
		t.Fatalf("milestone = %+v", got.Milestones[0])
	}
	// Twice is a no-op.
	if err := e.svc.AutoRelease(ctx, esc.ID, 0); err != nil || strings.Count(strings.Join(e.contract.calls, ","), "auto_release") != 1 {
		t.Fatalf("second auto_release: %v %v", err, e.contract.calls)
	}

	// A payout to an account that unregistered from the token is pending.
	chain.PendingPayouts = []domain.PendingPayout{{AccountID: member, Amount: "298500"}}
	delete(e.storage.registered, member)
	if err := e.svc.Sync(ctx, esc.ID); err != nil {
		t.Fatal(err)
	}
	due, _ = e.svc.FindDue(ctx, time.Minute, 10)
	if len(due.RetryPayout) != 1 || due.RetryPayout[0].AccountID != member {
		t.Fatalf("due payouts = %+v", due.RetryPayout)
	}
	if err := e.svc.RetryPayout(ctx, esc.ID, member); err != nil {
		t.Fatal(err)
	}
	if !e.storage.registered[member] || len(e.repo.escrow(esc.ID).PendingPayouts) != 0 {
		t.Fatal("payout not retried after registration")
	}
	if err := e.svc.RetryPayout(ctx, esc.ID, member); err != nil || strings.Count(strings.Join(e.contract.calls, ","), "retry_payout") != 1 {
		t.Fatalf("second retry: %v %v", err, e.contract.calls)
	}
}

func TestGetChecksGuestTokenEscrow(t *testing.T) {
	e := newEnv(t)
	esc := e.create(t, "freelancer")
	guest := &domain.GuestToken{EscrowID: "another", Wallet: domain.Wallet{Scheme: domain.SchemeEIP191, Address: clientEVM.Address()}}
	if _, err := e.svc.Get(context.Background(), esc.ID, domain.Requester{Guest: guest}); !isKind(err, domain.ErrUnauthorized) {
		t.Fatalf("token for another escrow: %v", err)
	}
	view, err := e.svc.Get(context.Background(), esc.ID, domain.Requester{})
	if err != nil || view.Escrow.ID != esc.ID {
		t.Fatalf("public view: %v", err)
	}
	if _, err := e.svc.Get(context.Background(), "nope", domain.Requester{}); !isKind(err, domain.ErrNotFound) {
		t.Fatalf("unknown escrow: %v", err)
	}
}

func TestDecodeDelegate(t *testing.T) {
	raw := delegateFor(freelancer, freelancerKey, contractID, submitCall("e", 0))
	got, err := escrow.DecodeDelegate(base64.StdEncoding.EncodeToString(raw))
	if err != nil || string(got) != string(raw) {
		t.Fatalf("DecodeDelegate: %v", err)
	}
	if _, err := escrow.DecodeDelegate("%%%"); !isKind(err, domain.ErrInvalidArgument) {
		t.Fatalf("bad base64: %v", err)
	}
}
