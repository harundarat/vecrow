//go:build e2e

// Package e2e_test runs the backend against a local NEAR sandbox with the
// real escrow and mock USDC contracts: `make test-e2e`. The api and the
// worker are wired by bootstrap from environment variables like the
// binaries, and use PostgreSQL, Redis and RabbitMQ from `make compose-up`.
// 1Click is a fake server shaped like its documentation; the test plays the
// solver that delivers the USDC.
package e2e_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/harundarat/vecrow/backend/internal/domain"
	"github.com/harundarat/vecrow/backend/internal/neartx"
	"github.com/harundarat/vecrow/backend/internal/walletsig/walletsigtest"
)

const (
	feeBPS = 50
	// reviewWindowSec is short so auto_release is due during the test, and
	// long enough for the manual steps on a submitted milestone.
	reviewWindowSec = 15
	appURL          = "http://localhost:5173"
	explorerTxURL   = "https://explorer.e2e/txns/"
	ethAsset        = "nep141:eth.omft.near"
)

// --- API shapes -------------------------------------------------------------

type eventView struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data"`
}

type txView struct {
	Hash       string      `json:"hash"`
	URL        string      `json:"url"`
	Method     string      `json:"method"`
	SignerID   string      `json:"signer_id"`
	SenderID   string      `json:"sender_id"`
	ReceiverID string      `json:"receiver_id"`
	Succeeded  bool        `json:"succeeded"`
	Error      string      `json:"error"`
	Events     []eventView `json:"events"`
}

func (tx txView) hasEvent(name string) bool {
	return slices.ContainsFunc(tx.Events, func(e eventView) bool { return e.Event == name })
}

type escrowView struct {
	ID          string `json:"id"`
	Link        string `json:"link"`
	ContractID  string `json:"contract_id"`
	Status      string `json:"status"`
	CreatorRole string `json:"creator_role"`
	FeeBPS      int    `json:"fee_bps"`
	Total       string `json:"total"`
	Milestones  []struct {
		Index         int     `json:"index"`
		Amount        string  `json:"amount"`
		Status        string  `json:"status"`
		SubmittedAtMS *uint64 `json:"submitted_at_ms"`
		RevisionCount int     `json:"revision_count"`
	} `json:"milestones"`
	Submission *struct {
		Status    string `json:"status"`
		Message   string `json:"message"`
		Nonce     string `json:"nonce"`
		Recipient string `json:"recipient"`
	} `json:"submission"`
	Balance        string `json:"balance"`
	PendingPayouts []struct {
		AccountID string `json:"account_id"`
		Amount    string `json:"amount"`
	} `json:"pending_payouts"`
	Transactions  []txView `json:"transactions"`
	RevisionNotes []struct {
		Milestone int    `json:"milestone"`
		Reason    string `json:"reason"`
		TxHash    string `json:"tx_hash"`
	} `json:"revision_notes"`
}

func (e escrowView) txs(method string) []txView {
	var out []txView
	for _, tx := range e.Transactions {
		if tx.Method == method {
			out = append(out, tx)
		}
	}
	return out
}

type termsView struct {
	Terms struct {
		DeadlineMS uint64 `json:"deadline_ms"`
		FeeBPS     int    `json:"fee_bps"`
	} `json:"terms"`
	Message   string `json:"message"`
	Nonce     string `json:"nonce"`
	Recipient string `json:"recipient"`
}

type submitView struct {
	Escrow     escrowView `json:"escrow"`
	GuestToken string     `json:"guest_token"`
}

type actionView struct {
	Escrow      escrowView `json:"escrow"`
	Transaction *txView    `json:"transaction"`
}

type fundingView struct {
	Status         string `json:"status"`
	OneClickStatus string `json:"oneclick_status"`
	EscrowStatus   string `json:"escrow_status"`
	Quote          struct {
		DepositAddress string `json:"deposit_address"`
	} `json:"quote"`
}

// --- Test world -------------------------------------------------------------

// world is the deployed contracts, the running backend and the accounts
// that act outside it.
type world struct {
	s        *sandbox
	app      *app
	oc       *fakeOneClick
	contract string
	usdc     string
	treasury string
	relayer  string
	solver   *signerAccount
	// minDeposit is the token's storage_balance_bounds().min.
	minDeposit *big.Int
	// secrets are guest tokens and signatures to look for in the logs.
	secrets []string
}

type signerAccount struct {
	id  string
	key ed25519.PrivateKey
}

func randKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func initCall(t *testing.T, args any) neartx.FunctionCall {
	return neartx.FunctionCall{MethodName: "new", Args: mustJSON(t, args), Gas: 100 * neartx.TGas}
}

func TestEscrowE2E(t *testing.T) {
	for _, k := range []string{"CACHE_URL", "QUEUE_URL"} {
		if os.Getenv(k) == "" {
			t.Fatalf("%s is not set; run `make compose-up` and `make test-e2e`", k)
		}
	}
	s := startSandbox(t)
	root := s.root.AccountID()
	acct := func(name string) string { return name + "." + root }
	w := &world{s: s, contract: acct("escrow"), usdc: acct("usdc"), treasury: acct("treasury"), relayer: acct("relayer")}

	// The token and the escrow contract, deployed and initialized as at a
	// real deploy, with a short review window.
	s.createAccount(t, w.usdc, near(20), randKey(t),
		neartx.DeployContract{Code: wasm(t, "mock-ft", "mock_ft.wasm")},
		initCall(t, map[string]any{"symbol": "USDC", "decimals": 6}))
	s.createAccount(t, w.treasury, near(5), randKey(t))
	s.createAccount(t, w.contract, near(30), randKey(t),
		neartx.DeployContract{Code: wasm(t, "escrow", "vecrow_escrow.wasm")},
		initCall(t, map[string]any{
			"usdc_contract_id": w.usdc, "treasury_account": w.treasury,
			"fee_bps": feeBPS, "review_window_sec": reviewWindowSec,
		}))
	relayerKey, storageKey := randKey(t), randKey(t)
	s.createAccount(t, w.relayer, near(50), relayerKey)
	storage := s.createAccount(t, acct("storage"), near(20), storageKey)

	var bounds struct {
		Min string `json:"min"`
	}
	s.view(t, w.usdc, "storage_balance_bounds", map[string]any{}, &bounds)
	w.minDeposit, _ = new(big.Int).SetString(bounds.Min, 10)
	// The contract and the treasury are registered at deploy.
	for _, id := range []string{w.contract, w.treasury} {
		s.call(t, storage, w.usdc, "storage_deposit", map[string]any{"account_id": id, "registration_only": true}, w.minDeposit)
	}
	// The solver delivers the swaps' USDC.
	w.solver = &signerAccount{id: acct("solver"), key: randKey(t)}
	solver := s.createAccount(t, w.solver.id, near(5), w.solver.key)
	s.call(t, solver, w.usdc, "storage_deposit", map[string]any{"account_id": w.solver.id, "registration_only": true}, w.minDeposit)
	s.call(t, solver, w.usdc, "mint", map[string]any{"account_id": w.solver.id, "amount": usdc(1_000_000).String()}, nil)

	w.oc = newFakeOneClick(t, w.usdc)
	w.app = startApp(t, map[string]string{
		"APP_ENV":               "development",
		"LOG_LEVEL":             "debug",
		"HTTP_ADDR":             freeAddr(t),
		"DATABASE_URL":          createDatabase(t),
		"SCHEDULER_INTERVAL":    "500ms",
		"AUTH_DOMAIN":           "localhost:5173",
		"NEAR_RPC_URL":          s.url,
		"NEAR_EXPLORER_TX_URL":  explorerTxURL,
		"ESCROW_CONTRACT_ID":    w.contract,
		"ESCROW_FEE_BPS":        strconv.Itoa(feeBPS),
		"ESCROW_TERMS_TTL":      "1h",
		"USDC_CONTRACT_ID":      w.usdc,
		"APP_URL":               appURL,
		"RELAYER_ACCOUNT_ID":    w.relayer,
		"RELAYER_PRIVATE_KEY":   neartx.FormatPrivateKey(relayerKey),
		"RELAYER_MAX_GAS_TGAS":  "100",
		"STORAGE_ACCOUNT_ID":    storage.AccountID(),
		"STORAGE_PRIVATE_KEY":   neartx.FormatPrivateKey(storageKey),
		"ONECLICK_URL":          w.oc.srv.URL,
		"ONECLICK_API_KEY":      oneClickKey,
		"ONECLICK_SLIPPAGE_BPS": "100",
		// GET funding never reads 1Click itself, so a SUCCESS there comes
		// from the watcher job.
		"ONECLICK_REFRESH_AFTER": "1h",
		"JOB_SYNC_INTERVAL":      "2s",
		"JOB_FUNDING_INTERVAL":   "1s",
		"JOB_LEASE_TTL":          "30s",
	}, appKeys{relayer: relayerKey, storage: storageKey})

	t.Run("freelancer creates, NEAR approver", w.freelancerCreates)
	t.Run("client creates, EVM approver", w.clientCreates)
	w.app.checkNoSecretsLogged(t, w.secrets...)
}

// --- Helpers ----------------------------------------------------------------

func bearer(token string) []string { return []string{"Authorization", "Bearer " + token} }

func guestHeader(token string) []string { return []string{"X-Guest-Token", token} }

// loginNEAR logs in with a NEAR wallet's NEP-413 signature.
func (w *world) loginNEAR(t *testing.T, accountID string, key ed25519.PrivateKey) string {
	t.Helper()
	return w.login(t, "nep413", accountID, neartx.PublicKeyOf(key).String(), func(msg string, nonce [32]byte, recipient string) string {
		return walletsigtest.SignNEP413(key, msg, nonce, recipient)
	})
}

// loginEVM logs in with an EVM wallet's EIP-191 signature.
func (w *world) loginEVM(t *testing.T, k walletsigtest.EVMKey) string {
	t.Helper()
	return w.login(t, "eip191", k.Address(), "", func(msg string, _ [32]byte, _ string) string { return k.SignEIP191(msg) })
}

func (w *world) login(t *testing.T, scheme, address, publicKey string, sign func(string, [32]byte, string) string) string {
	t.Helper()
	var n struct {
		Nonce     string `json:"nonce"`
		Message   string `json:"message"`
		Recipient string `json:"recipient"`
	}
	w.app.must(t, http.StatusCreated, http.MethodPost, "/auth/nonce", map[string]string{"scheme": scheme, "address": address}, &n)
	var out struct {
		Token string `json:"token"`
	}
	w.app.must(t, http.StatusOK, http.MethodPost, "/auth/verify", map[string]string{
		"nonce": n.Nonce, "signature": sign(n.Message, decodeNonce(t, n.Nonce), n.Recipient), "public_key": publicKey,
	}, &out)
	w.secrets = append(w.secrets, out.Token)
	return out.Token
}

func decodeNonce(t *testing.T, s string) [32]byte {
	t.Helper()
	var n [32]byte
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 32 {
		t.Fatalf("nonce %q", s)
	}
	copy(n[:], b)
	return n
}

func (w *world) get(t *testing.T, id string) escrowView {
	t.Helper()
	var e escrowView
	w.app.must(t, http.StatusOK, http.MethodGet, "/escrows/"+id, nil, &e)
	return e
}

// waitFor polls GET /escrows/:id until cond holds.
func (w *world) waitFor(t *testing.T, id string, timeout time.Duration, what string, cond func(escrowView) bool) escrowView {
	t.Helper()
	var e escrowView
	eventually(t, timeout, what, func() bool {
		e = w.get(t, id)
		return cond(e)
	})
	return e
}

// waitCreated waits until the creation job has sent create_escrow and marked
// the escrow Created, and checks get_escrow.
func (w *world) waitCreated(t *testing.T, id string) escrowView {
	t.Helper()
	e := w.waitFor(t, id, 60*time.Second, "escrow Created", func(e escrowView) bool {
		return e.Status == "Created" && len(e.txs("create_escrow")) > 0
	})
	if got := w.chainStatus(t, id); got != "Created" {
		t.Fatalf("get_escrow status %q", got)
	}
	return e
}

// myEscrows returns GET /me/escrows with query as escrow ID → roles.
func (w *world) myEscrows(t *testing.T, token, query string) map[string][]string {
	t.Helper()
	var out struct {
		Escrows []struct {
			ID    string   `json:"id"`
			Roles []string `json:"roles"`
		} `json:"escrows"`
	}
	w.app.must(t, http.StatusOK, http.MethodGet, "/me/escrows"+query, nil, &out, bearer(token)...)
	m := make(map[string][]string, len(out.Escrows))
	for _, e := range out.Escrows {
		m[e.ID] = e.Roles
	}
	return m
}

func (w *world) chainStatus(t *testing.T, id string) string {
	t.Helper()
	var e *struct {
		Status string `json:"status"`
	}
	w.s.view(t, w.contract, "get_escrow", map[string]string{"escrow_id": id}, &e)
	if e == nil {
		return ""
	}
	return e.Status
}

func (w *world) usdcBalance(t *testing.T, accountID string) *big.Int {
	t.Helper()
	var b string
	w.s.view(t, w.usdc, "ft_balance_of", map[string]string{"account_id": accountID}, &b)
	n, _ := new(big.Int).SetString(b, 10)
	return n
}

// zeroAccount creates an account with 0 NEAR and a full access key, like a
// new wallet user's.
func (w *world) zeroAccount(t *testing.T, name string) signerAccount {
	t.Helper()
	a := signerAccount{id: name + "." + w.s.root.AccountID(), key: randKey(t)}
	w.s.createAccount(t, a.id, new(big.Int), a.key)
	if b := w.s.nearBalance(t, a.id); b.Sign() != 0 {
		t.Fatalf("%s has %s yoctoNEAR", a.id, b)
	}
	return a
}

// approverAction is the approver's signed authorization of an action on a
// milestone, signed by sign.
func (w *world) approverAction(t *testing.T, id string, action domain.ApproverAction, milestone uint32, sign func(msg string, nonce [32]byte) string) map[string]any {
	t.Helper()
	var nonce [32]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	deadline := uint64(time.Now().Add(10 * time.Minute).UnixMilli())
	msg := domain.MilestoneActionMessage(action, w.contract, id, milestone, nonce, deadline)
	sig := sign(msg, nonce)
	w.secrets = append(w.secrets, sig)
	return map[string]any{"nonce": hex.EncodeToString(nonce[:]), "deadline_ms": deadline, "signature": sig}
}

// submit relays the freelancer's submit_milestone.
func (w *world) submit(t *testing.T, id string, f signerAccount, milestone int) actionView {
	t.Helper()
	del := w.s.delegate(t, f.id, f.key, w.contract, "submit_milestone",
		map[string]any{"escrow_id": id, "milestone": milestone}, 30*neartx.TGas, nil)
	var r actionView
	w.app.must(t, http.StatusOK, http.MethodPost, fmt.Sprintf("/escrows/%s/milestones/%d/submit", id, milestone),
		map[string]string{"delegate_action": del}, &r)
	w.checkRelayed(t, r, f.id, "submit_milestone", "milestone_submitted")
	if got := r.Escrow.Milestones[milestone].Status; got != "Submitted" {
		t.Fatalf("milestone %d is %s after submit", milestone, got)
	}
	return r
}

// checkRelayed checks a transaction the relayer sent on the freelancer's
// behalf (NEP-366): signed and paid by the relayer, sent by the freelancer.
func (w *world) checkRelayed(t *testing.T, r actionView, sender, method, event string) {
	t.Helper()
	tx := r.Transaction
	if tx == nil || !tx.Succeeded || tx.Method != method || tx.SignerID != w.relayer || tx.SenderID != sender ||
		tx.ReceiverID != w.contract || tx.URL != explorerTxURL+tx.Hash || !tx.hasEvent(event) {
		t.Fatalf("%s transaction = %+v", method, tx)
	}
}

// fund requests a 1Click quote, then plays the solver: it delivers the
// escrow's total to the contract with the escrow ID as msg and reports the
// swap done. The watcher job then sees SUCCESS.
func (w *world) fund(t *testing.T, id, total string, auth []string) {
	t.Helper()
	type tokenView struct {
		AssetID string `json:"asset_id"`
	}
	var tokens struct {
		Tokens []tokenView `json:"tokens"`
	}
	w.app.must(t, http.StatusOK, http.MethodGet, "/tokens", nil, &tokens)
	if !slices.ContainsFunc(tokens.Tokens, func(tk tokenView) bool { return tk.AssetID == ethAsset }) {
		t.Fatalf("tokens = %+v", tokens)
	}

	quoteBody := map[string]string{"origin_asset": ethAsset, "refund_to": "0x1111111111111111111111111111111111111111"}
	if res := w.app.do(t, http.MethodPost, "/escrows/"+id+"/quote", quoteBody); res.status != http.StatusUnauthorized {
		t.Fatalf("quote without credentials: %d %s", res.status, res.body)
	}
	var q struct {
		DepositAddress string `json:"deposit_address"`
		AmountOut      string `json:"amount_out"`
		Status         string `json:"status"`
	}
	w.app.must(t, http.StatusCreated, http.MethodPost, "/escrows/"+id+"/quote", quoteBody, &q, auth...)
	if q.AmountOut != total || q.Status != "PENDING" || q.DepositAddress == "" {
		t.Fatalf("quote = %+v", q)
	}
	req := w.oc.quote(q.DepositAddress)
	for k, want := range map[string]any{
		"swapType": "EXACT_OUTPUT", "originAsset": ethAsset, "destinationAsset": "nep141:" + w.usdc,
		"amount": total, "recipient": w.contract, "recipientType": "DESTINATION_CHAIN",
		"customRecipientMsg": id, "depositType": "ORIGIN_CHAIN", "refundType": "ORIGIN_CHAIN",
		"refundTo": quoteBody["refund_to"], "dry": false,
	} {
		if req[k] != want {
			t.Errorf("1Click quote request %s = %v, want %v", k, req[k], want)
		}
	}

	var f fundingView
	w.app.must(t, http.StatusOK, http.MethodGet, "/escrows/"+id+"/funding", nil, &f, auth...)
	if f.Status != "PENDING" || f.EscrowStatus != "Created" {
		t.Fatalf("funding before the deposit = %+v", f)
	}

	solver := w.s.signer(w.solver.id, w.solver.key)
	w.s.call(t, solver, w.usdc, "ft_transfer_call",
		map[string]string{"receiver_id": w.contract, "amount": total, "msg": id}, big.NewInt(1))
	w.oc.setStatus(q.DepositAddress, "SUCCESS")

	eventually(t, 30*time.Second, "funding SUCCESS from the watcher", func() bool {
		w.app.must(t, http.StatusOK, http.MethodGet, "/escrows/"+id+"/funding", nil, &f, auth...)
		return f.Status == "SUCCESS" && f.OneClickStatus == "SUCCESS" && f.EscrowStatus == "Funded"
	})
	if w.oc.pollCount(q.DepositAddress) == 0 {
		t.Error("the watcher never polled 1Click")
	}
	if got := w.chainStatus(t, id); got != "Funded" {
		t.Fatalf("get_escrow status %q after funding", got)
	}
}

// --- Freelancer as creator --------------------------------------------------

// freelancerCreates: a freelancer creates the escrow, a client with a NEAR
// wallet joins through the link. Funding, a revision, an approval with a
// pending payout retried after storage registration, an auto-release and a
// refund follow. Freelancer and client hold 0 NEAR throughout.
func (w *world) freelancerCreates(t *testing.T) {
	f, c := w.zeroAccount(t, "fa"), w.zeroAccount(t, "ca")
	memberKey := randKey(t)
	memberID := "ma." + w.s.root.AccountID()
	member := w.s.createAccount(t, memberID, near(5), memberKey)
	m0, m1, m2 := usdc(1000), usdc(500), usdc(300)
	total := new(big.Int).Add(m0, new(big.Int).Add(m1, m2)).String()

	token := w.loginNEAR(t, f.id, f.key)
	var e escrowView
	w.app.must(t, http.StatusCreated, http.MethodPost, "/escrows", map[string]any{
		"role":       "freelancer",
		"milestones": []string{m0.String(), m1.String(), m2.String()},
		"freelancer": f.id,
		"split":      []map[string]any{{"account_id": f.id, "bps": 7000}, {"account_id": memberID, "bps": 3000}},
	}, &e, bearer(token)...)
	id := e.ID
	if e.Status != "Open" || e.Link != appURL+"/escrows/"+id || e.ContractID != w.contract || e.Total != total || e.FeeBPS != feeBPS {
		t.Fatalf("created escrow = %+v", e)
	}
	// It is listed under the creator's role at once, filtered by role and
	// status.
	if roles := w.myEscrows(t, token, "?role=freelancer&status=Open")[id]; !slices.Equal(roles, []string{"freelancer"}) {
		t.Errorf("my Open escrows as freelancer: roles %v", roles)
	}
	for _, q := range []string{"?role=client", "?status=Created"} {
		if _, ok := w.myEscrows(t, token, q)[id]; ok {
			t.Errorf("my escrows%s lists the escrow", q)
		}
	}

	// The client opens the link without logging in and signs the final terms.
	cPub := neartx.PublicKeyOf(c.key).String()
	details := map[string]any{
		"approver":       map[string]string{"scheme": "nep413", "account_id": c.id, "public_key": cPub},
		"refund_account": c.id,
	}
	var terms termsView
	w.app.must(t, http.StatusOK, http.MethodPost, "/escrows/"+id+"/terms", details, &terms)
	if terms.Recipient != w.contract || terms.Terms.FeeBPS != feeBPS {
		t.Fatalf("terms = %+v", terms)
	}
	sig := walletsigtest.SignNEP413(c.key, terms.Message, decodeNonce(t, terms.Nonce), terms.Recipient)
	submitBody := map[string]any{"deadline_ms": terms.Terms.DeadlineMS, "signature": sig, "public_key": cPub}
	for k, v := range details {
		submitBody[k] = v
	}
	var sub submitView
	w.app.must(t, http.StatusCreated, http.MethodPost, "/escrows/"+id+"/counterparty", submitBody, &sub)
	guest := sub.GuestToken
	w.secrets = append(w.secrets, guest, sig)
	if sub.Escrow.Status != "PendingApproval" || guest == "" {
		t.Fatalf("submission = %+v", sub)
	}

	// The creator signs the same terms with their own wallet.
	msg := sub.Escrow.Submission.Message
	fsig := walletsigtest.SignNEP413(f.key, msg, decodeNonce(t, sub.Escrow.Submission.Nonce), w.contract)
	w.secrets = append(w.secrets, fsig)
	w.app.must(t, http.StatusOK, http.MethodPost, "/escrows/"+id+"/counterparty/approve",
		map[string]string{"signature": fsig, "public_key": neartx.PublicKeyOf(f.key).String()}, &e, bearer(token)...)
	if e.Status != "Approved" {
		t.Fatalf("approved escrow = %+v", e)
	}

	// The worker registers storage for the payout accounts, then sends
	// create_escrow with both signatures through the relayer.
	e = w.waitCreated(t, id)
	creates := e.txs("create_escrow")
	if len(creates) != 1 || !creates[0].Succeeded || creates[0].SignerID != w.relayer || !creates[0].hasEvent("escrow_created") ||
		creates[0].URL != explorerTxURL+creates[0].Hash {
		t.Fatalf("create_escrow transactions = %+v", creates)
	}
	registered := map[string]bool{}
	for _, tx := range e.txs("storage_deposit") {
		if !tx.Succeeded || tx.ReceiverID != w.usdc {
			t.Errorf("storage_deposit = %+v", tx)
		}
		registered[tx.Hash] = true
	}
	if len(registered) != 3 { // freelancer, member, refund account
		t.Errorf("storage_deposit transactions = %d, want 3", len(registered))
	}

	if roles := w.myEscrows(t, token, "?status=Created")[id]; !slices.Equal(roles, []string{"freelancer"}) {
		t.Errorf("my Created escrows: roles %v", roles)
	}
	// The client joined without an account; logging in with the approver
	// wallet shows the escrow under the client role.
	clientToken := w.loginNEAR(t, c.id, c.key)
	if roles := w.myEscrows(t, clientToken, "?role=client")[id]; !slices.Equal(roles, []string{"client"}) {
		t.Errorf("client's escrows: roles %v", roles)
	}

	// The client funds through 1Click with the guest token.
	w.fund(t, id, total, guestHeader(guest))

	// The member leaves the token, so their next payout fails and is kept
	// as pending by the contract.
	w.s.call(t, member, w.usdc, "storage_unregister", map[string]any{"force": false}, big.NewInt(1))

	// The relayer only forwards the freelancer's calls to the contract.
	bad := w.s.delegate(t, f.id, f.key, w.usdc, "ft_transfer", map[string]any{"receiver_id": f.id, "amount": "1"}, 30*neartx.TGas, big.NewInt(1))
	if res := w.app.do(t, http.MethodPost, "/escrows/"+id+"/refund", map[string]string{"delegate_action": bad}); res.status != http.StatusForbidden {
		t.Fatalf("delegate to the token: %d %s", res.status, res.body)
	}
	greedy := w.s.delegate(t, f.id, f.key, w.contract, "refund_unsubmitted", map[string]any{"escrow_id": id}, 250*neartx.TGas, nil)
	if res := w.app.do(t, http.MethodPost, "/escrows/"+id+"/refund", map[string]string{"delegate_action": greedy}); res.status != http.StatusForbidden {
		t.Fatalf("delegate over the gas cap: %d %s", res.status, res.body)
	}
	forged := w.s.delegate(t, w.solver.id, w.solver.key, w.contract, "refund_unsubmitted", map[string]any{"escrow_id": id}, 50*neartx.TGas, nil)
	if res := w.app.do(t, http.MethodPost, "/escrows/"+id+"/refund", map[string]string{"delegate_action": forged}); res.status != http.StatusForbidden {
		t.Fatalf("delegate from another account: %d %s", res.status, res.body)
	}

	signC := func(msg string, nonce [32]byte) string {
		return walletsigtest.SignNEP413(c.key, msg, nonce, w.contract)
	}

	// Milestone 0: submit, revision with a reason, submit again, approve.
	w.submit(t, id, f, 0)
	rev := w.approverAction(t, id, domain.ActionRequestRevision, 0, signC)
	rev["reason"] = "Please use the brand colors"
	var r actionView
	w.app.must(t, http.StatusOK, http.MethodPost, "/escrows/"+id+"/milestones/0/revision", rev, &r)
	if r.Transaction == nil || !r.Transaction.Succeeded || !r.Transaction.hasEvent("revision_requested") ||
		r.Escrow.Milestones[0].Status != "RevisionRequested" || r.Escrow.Milestones[0].RevisionCount != 1 {
		t.Fatalf("revision = %+v", r)
	}
	w.submit(t, id, f, 0)
	w.app.must(t, http.StatusOK, http.MethodPost, "/escrows/"+id+"/milestones/0/approve",
		w.approverAction(t, id, domain.ActionApproveMilestone, 0, signC), &r)
	if r.Transaction == nil || !r.Transaction.Succeeded || !r.Transaction.hasEvent("milestone_released") ||
		r.Escrow.Milestones[0].Status != "Released" {
		t.Fatalf("approve = %+v", r)
	}
	_, memberShare0 := shares(m0, 3000)
	if len(r.Escrow.PendingPayouts) != 1 || r.Escrow.PendingPayouts[0].AccountID != memberID ||
		r.Escrow.PendingPayouts[0].Amount != memberShare0.String() {
		t.Fatalf("pending payouts after approve = %+v", r.Escrow.PendingPayouts)
	}

	// The payout retry job registers the member again (the storage account
	// pays), then calls retry_payout.
	e = w.waitFor(t, id, 60*time.Second, "pending payout retried", func(e escrowView) bool {
		return len(e.PendingPayouts) == 0 && len(e.txs("retry_payout")) > 0
	})
	retry := e.txs("retry_payout")[0]
	if !retry.Succeeded || retry.SignerID != w.relayer || !retry.hasEvent("payout") {
		t.Fatalf("retry_payout = %+v", retry)
	}
	if got := w.usdcBalance(t, memberID); got.Cmp(memberShare0) != 0 {
		t.Fatalf("member balance %s after retry, want %s", got, memberShare0)
	}
	if len(e.txs("storage_deposit")) != 4 {
		t.Errorf("storage_deposit transactions = %d, want 4", len(e.txs("storage_deposit")))
	}
	if len(e.RevisionNotes) != 1 || e.RevisionNotes[0].Reason != "Please use the brand colors" || e.RevisionNotes[0].Milestone != 0 {
		t.Errorf("revision notes = %+v", e.RevisionNotes)
	}

	// Milestone 1: submitted, then released by the worker after the review
	// window.
	w.submit(t, id, f, 1)
	// GET /escrows/:id reads the contract, so the release can show before
	// the job has stored its transaction.
	e = w.waitFor(t, id, (reviewWindowSec+60)*time.Second, "auto_release", func(e escrowView) bool {
		return e.Milestones[1].Status == "Released" && len(e.txs("auto_release")) > 0
	})
	auto := e.txs("auto_release")
	if len(auto) != 1 || !auto[0].Succeeded || auto[0].SignerID != w.relayer || !auto[0].hasEvent("milestone_released") {
		t.Fatalf("auto_release = %+v", auto)
	}

	// Milestone 2 was never submitted: the freelancer refunds it to the
	// client's refund account.
	refund := w.s.delegate(t, f.id, f.key, w.contract, "refund_unsubmitted", map[string]any{"escrow_id": id}, 100*neartx.TGas, nil)
	w.app.must(t, http.StatusOK, http.MethodPost, "/escrows/"+id+"/refund", map[string]string{"delegate_action": refund}, &r)
	w.checkRelayed(t, r, f.id, "refund_unsubmitted", "milestones_refunded")
	if r.Escrow.Status != "Completed" || r.Escrow.Milestones[2].Status != "Refunded" {
		t.Fatalf("after refund = %+v", r.Escrow)
	}
	if got := w.chainStatus(t, id); got != "Completed" {
		t.Fatalf("get_escrow status %q", got)
	}

	// Balances: fee to the treasury, 70/30 split of the rest, refund to the
	// client; freelancer and client still hold 0 NEAR.
	f0, mem0 := shares(m0, 3000)
	f1, mem1 := shares(m1, 3000)
	for acct, want := range map[string]*big.Int{
		f.id:       new(big.Int).Add(f0, f1),
		memberID:   new(big.Int).Add(mem0, mem1),
		c.id:       m2,
		w.treasury: new(big.Int).Add(fee(m0), fee(m1)),
	} {
		if got := w.usdcBalance(t, acct); got.Cmp(want) != 0 {
			t.Errorf("USDC of %s = %s, want %s", acct, got, want)
		}
	}
	for _, a := range []string{f.id, c.id} {
		if b := w.s.nearBalance(t, a); b.Sign() != 0 {
			t.Errorf("%s holds %s yoctoNEAR, want 0", a, b)
		}
	}
	for _, tx := range w.get(t, id).Transactions {
		if tx.URL != explorerTxURL+tx.Hash || tx.Hash == "" {
			t.Errorf("transaction link = %+v", tx)
		}
	}
}

// --- Client as creator ------------------------------------------------------

// clientCreates: a client with an EVM wallet creates the escrow, a
// freelancer joins through the link, then funding, a submission and the
// approval complete it. Freelancer and client hold 0 NEAR throughout.
func (w *world) clientCreates(t *testing.T) {
	f, refundAcct := w.zeroAccount(t, "fb"), w.zeroAccount(t, "cb")
	evm := walletsigtest.NewEVMKey(77)
	amount := usdc(250)

	token := w.loginEVM(t, evm)
	var e escrowView
	w.app.must(t, http.StatusCreated, http.MethodPost, "/escrows", map[string]any{
		"role":           "client",
		"milestones":     []string{amount.String()},
		"approver":       map[string]string{"scheme": "eip191", "address": evm.Address()},
		"refund_account": refundAcct.id,
	}, &e, bearer(token)...)
	id := e.ID
	if e.Status != "Open" || e.CreatorRole != "client" {
		t.Fatalf("created escrow = %+v", e)
	}

	// The freelancer joins with their NEAR wallet, without logging in.
	fPub := neartx.PublicKeyOf(f.key).String()
	details := map[string]any{"freelancer": f.id, "split": []map[string]any{{"account_id": f.id, "bps": 10000}}}
	var terms termsView
	w.app.must(t, http.StatusOK, http.MethodPost, "/escrows/"+id+"/terms", details, &terms)
	sig := walletsigtest.SignNEP413(f.key, terms.Message, decodeNonce(t, terms.Nonce), terms.Recipient)
	body := map[string]any{"deadline_ms": terms.Terms.DeadlineMS, "signature": sig, "public_key": fPub}
	for k, v := range details {
		body[k] = v
	}
	var sub submitView
	w.app.must(t, http.StatusCreated, http.MethodPost, "/escrows/"+id+"/counterparty", body, &sub)
	w.secrets = append(w.secrets, sub.GuestToken, sig)

	// The freelancer's guest token can be renewed with a fresh login
	// signature from the same wallet.
	var n struct {
		Nonce     string `json:"nonce"`
		Message   string `json:"message"`
		Recipient string `json:"recipient"`
	}
	w.app.must(t, http.StatusCreated, http.MethodPost, "/auth/nonce", map[string]string{"scheme": "nep413", "address": f.id}, &n)
	var renewed struct {
		GuestToken string `json:"guest_token"`
	}
	w.app.must(t, http.StatusCreated, http.MethodPost, "/escrows/"+id+"/guest-token", map[string]string{
		"nonce": n.Nonce, "signature": walletsigtest.SignNEP413(f.key, n.Message, decodeNonce(t, n.Nonce), n.Recipient), "public_key": fPub,
	}, &renewed)
	if renewed.GuestToken == "" || renewed.GuestToken == sub.GuestToken {
		t.Fatalf("renewed guest token = %+v", renewed)
	}
	w.secrets = append(w.secrets, renewed.GuestToken)
	w.app.must(t, http.StatusOK, http.MethodGet, "/escrows/"+id, nil, nil, guestHeader(renewed.GuestToken)...)

	// The client approves with an EIP-191 signature over the same terms.
	csig := evm.SignEIP191(sub.Escrow.Submission.Message)
	w.secrets = append(w.secrets, csig)
	w.app.must(t, http.StatusOK, http.MethodPost, "/escrows/"+id+"/counterparty/approve", map[string]string{"signature": csig}, &e, bearer(token)...)
	if e.Status != "Approved" {
		t.Fatalf("approved escrow = %+v", e)
	}
	w.waitCreated(t, id)
	// The freelancer joined without an account; logging in with the same
	// wallet shows the escrow under the freelancer role.
	if roles := w.myEscrows(t, w.loginNEAR(t, f.id, f.key), "?role=freelancer")[id]; !slices.Equal(roles, []string{"freelancer"}) {
		t.Errorf("freelancer's escrows: roles %v", roles)
	}

	// The client funds with their session, which owns the approver wallet.
	w.fund(t, id, amount.String(), bearer(token))

	w.submit(t, id, f, 0)
	signEVM := func(msg string, _ [32]byte) string { return evm.SignEIP191(msg) }
	var r actionView
	w.app.must(t, http.StatusOK, http.MethodPost, "/escrows/"+id+"/milestones/0/approve",
		w.approverAction(t, id, domain.ActionApproveMilestone, 0, signEVM), &r)
	if r.Transaction == nil || !r.Transaction.Succeeded || r.Escrow.Status != "Completed" || r.Escrow.Milestones[0].Status != "Released" {
		t.Fatalf("approve = %+v", r)
	}
	share, _ := shares(amount, 0)
	if got := w.usdcBalance(t, f.id); got.Cmp(share) != 0 {
		t.Errorf("freelancer USDC = %s, want %s", got, share)
	}
	for _, a := range []string{f.id, refundAcct.id} {
		if b := w.s.nearBalance(t, a); b.Sign() != 0 {
			t.Errorf("%s holds %s yoctoNEAR, want 0", a, b)
		}
	}
}

// --- Amounts ----------------------------------------------------------------

// fee is the contract's fee on a milestone: floor(amount × fee_bps / 10,000).
func fee(amount *big.Int) *big.Int {
	return new(big.Int).Div(new(big.Int).Mul(amount, big.NewInt(feeBPS)), big.NewInt(10_000))
}

// shares splits a milestone after the fee between the freelancer (first
// share, with the rounding remainder) and a member with memberBPS.
func shares(amount *big.Int, memberBPS int64) (freelancer, member *big.Int) {
	rest := new(big.Int).Sub(amount, fee(amount))
	member = new(big.Int).Div(new(big.Int).Mul(rest, big.NewInt(memberBPS)), big.NewInt(10_000))
	return new(big.Int).Sub(rest, member), member
}
