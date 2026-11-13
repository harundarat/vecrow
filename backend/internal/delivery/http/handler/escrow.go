package handler

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/harundarat/vecrow/backend/internal/delivery/http/middleware"
	"github.com/harundarat/vecrow/backend/internal/delivery/http/response"
	"github.com/harundarat/vecrow/backend/internal/domain"
	"github.com/harundarat/vecrow/backend/internal/usecase/auth"
	"github.com/harundarat/vecrow/backend/internal/usecase/escrow"
)

// EscrowUseCase is the use case behind the /escrows endpoints.
type EscrowUseCase interface {
	ContractID() string
	Link(id string) string
	Create(ctx context.Context, in escrow.CreateInput) (domain.Escrow, error)
	Get(ctx context.Context, id string, req domain.Requester) (escrow.View, error)
	BuildTerms(ctx context.Context, id string, in escrow.DetailsInput) (escrow.Terms, error)
	SubmitCounterparty(ctx context.Context, id string, in escrow.SubmitInput) (escrow.SubmitResult, error)
	Approve(ctx context.Context, accountID, id string, in escrow.ApproveInput) (domain.Escrow, error)
	Reject(ctx context.Context, accountID, id string) (domain.Escrow, error)
	Cancel(ctx context.Context, id string, in escrow.CancelInput) (escrow.ActionResult, error)
	RenewGuestToken(ctx context.Context, id string, w domain.Wallet) (string, time.Time, error)
	SubmitMilestone(ctx context.Context, id string, milestone uint32, delegate []byte) (escrow.ActionResult, error)
	ApproveMilestone(ctx context.Context, id string, milestone uint32, a escrow.ApproverAction) (escrow.ActionResult, error)
	RequestRevision(ctx context.Context, id string, milestone uint32, a escrow.ApproverAction, reason string) (escrow.ActionResult, error)
	RefundUnsubmitted(ctx context.Context, id string, delegate []byte) (escrow.ActionResult, error)
	ListMine(ctx context.Context, accountID, role, status string) ([]escrow.AccountEscrow, error)
	UpdateInvitation(ctx context.Context, accountID, id string, in *escrow.InvitationInput) (domain.Escrow, error)
	Payouts(ctx context.Context, id string, req domain.Requester) ([]escrow.Payout, error)
}

// WalletProver proves control of a wallet with a signed login nonce.
type WalletProver interface {
	Prove(ctx context.Context, p auth.Proof) (domain.Wallet, error)
}

// EscrowHandler serves the /escrows endpoints and GET /me/escrows.
type EscrowHandler struct {
	escrows EscrowUseCase
	wallets WalletProver
	// explorerTxURL prefixes a transaction hash to link to it.
	explorerTxURL string
}

func NewEscrowHandler(escrows EscrowUseCase, wallets WalletProver, explorerTxURL string) *EscrowHandler {
	return &EscrowHandler{escrows: escrows, wallets: wallets, explorerTxURL: explorerTxURL}
}

// --- Requests ---------------------------------------------------------------

type splitShareJSON struct {
	AccountID string `json:"account_id" binding:"required"`
	BPS       uint16 `json:"bps"`
}

type approverJSON struct {
	Scheme    string `json:"scheme" binding:"required"`
	AccountID string `json:"account_id,omitempty"`
	PublicKey string `json:"public_key,omitempty"`
	Address   string `json:"address,omitempty"`
}

// detailsRequest is one party's own details: freelancer and split, or
// approver and refund_account.
type detailsRequest struct {
	Freelancer    string           `json:"freelancer"`
	Split         []splitShareJSON `json:"split" binding:"omitempty,dive"`
	Approver      *approverJSON    `json:"approver"`
	RefundAccount string           `json:"refund_account"`
}

func (r detailsRequest) input() escrow.DetailsInput {
	var in escrow.DetailsInput
	if r.Freelancer != "" || r.Split != nil {
		f := &escrow.FreelancerInput{Account: r.Freelancer}
		for _, s := range r.Split {
			f.Split = append(f.Split, domain.SplitShare{AccountID: s.AccountID, BPS: s.BPS})
		}
		in.Freelancer = f
	}
	if r.Approver != nil || r.RefundAccount != "" {
		c := &escrow.ClientInput{RefundAccount: r.RefundAccount}
		if a := r.Approver; a != nil {
			c.Approver = escrow.ApproverInput{Scheme: a.Scheme, AccountID: a.AccountID, PublicKey: a.PublicKey, Address: a.Address}
		}
		in.Client = c
	}
	return in
}

// invitationRequest names the other party by exactly one of username,
// email and wallet (R-15).
type invitationRequest struct {
	Username string      `json:"username"`
	Email    string      `json:"email"`
	Wallet   *walletJSON `json:"wallet"`
}

func (r *invitationRequest) input() *escrow.InvitationInput {
	if r == nil {
		return nil
	}
	in := &escrow.InvitationInput{Username: r.Username, Email: r.Email}
	if r.Wallet != nil {
		in.Wallet = &escrow.WalletInput{Scheme: r.Wallet.Scheme, Address: r.Wallet.Address}
	}
	return in
}

type createEscrowRequest struct {
	Role       string   `json:"role" binding:"required"`
	Milestones []string `json:"milestones" binding:"required"`
	detailsRequest
	Invitation *invitationRequest `json:"invitation"`
}

// updateInvitationRequest replaces the invitation; null removes it. The
// field is required so an empty body never removes an invitation.
type updateInvitationRequest struct {
	Invitation json.RawMessage `json:"invitation"`
}

type submitRequest struct {
	detailsRequest
	DeadlineMS uint64 `json:"deadline_ms" binding:"required"`
	Signature  string `json:"signature" binding:"required"`
	PublicKey  string `json:"public_key"`
}

type approveRequest struct {
	Signature string `json:"signature" binding:"required"`
	PublicKey string `json:"public_key"`
}

// approverActionRequest is the approver's signed authorization of an action.
type approverActionRequest struct {
	Nonce      string `json:"nonce" binding:"required"`
	DeadlineMS uint64 `json:"deadline_ms" binding:"required"`
	Signature  string `json:"signature" binding:"required"`
}

func (r approverActionRequest) action() escrow.ApproverAction {
	return escrow.ApproverAction{Nonce: r.Nonce, DeadlineMS: r.DeadlineMS, Signature: r.Signature}
}

type revisionRequest struct {
	approverActionRequest
	Reason string `json:"reason" binding:"required"`
}

// delegateRequest carries the freelancer's signed delegate action (NEP-366),
// base64 Borsh as wallets return it.
type delegateRequest struct {
	DelegateAction string `json:"delegate_action" binding:"required"`
}

type cancelRequest struct {
	DelegateAction string `json:"delegate_action"`
	Nonce          string `json:"nonce"`
	DeadlineMS     uint64 `json:"deadline_ms"`
	Signature      string `json:"signature"`
}

// --- Responses --------------------------------------------------------------

type walletJSON struct {
	Scheme  string `json:"scheme"`
	Address string `json:"address"`
}

type freelancerJSON struct {
	Account string                  `json:"account"`
	Split   []domain.SplitShareJSON `json:"split"`
}

type clientJSON struct {
	Approver      domain.ApproverJSON `json:"approver"`
	RefundAccount string              `json:"refund_account"`
}

type milestoneJSON struct {
	Index           int     `json:"index"`
	Amount          string  `json:"amount"`
	Status          string  `json:"status"`
	SubmittedAtMS   *uint64 `json:"submitted_at_ms"`
	AutoReleaseAtMS *uint64 `json:"auto_release_at_ms"`
	RevisionCount   int     `json:"revision_count"`
}

type submissionJSON struct {
	ID     string           `json:"id"`
	Status string           `json:"status"`
	Terms  domain.TermsJSON `json:"terms"`
	// Message, Nonce and Recipient are what the creator signs to approve.
	Message   string     `json:"message"`
	Nonce     string     `json:"nonce"`
	Recipient string     `json:"recipient"`
	Wallet    walletJSON `json:"wallet"`
	CreatedAt time.Time  `json:"created_at"`
}

type pendingPayoutJSON struct {
	AccountID string `json:"account_id"`
	Amount    string `json:"amount"`
}

type eventJSON struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data"`
}

type transactionJSON struct {
	Hash       string      `json:"hash"`
	URL        string      `json:"url,omitempty"`
	Method     string      `json:"method"`
	SignerID   string      `json:"signer_id"`
	SenderID   string      `json:"sender_id"`
	ReceiverID string      `json:"receiver_id"`
	Succeeded  bool        `json:"succeeded"`
	Error      string      `json:"error,omitempty"`
	Events     []eventJSON `json:"events"`
	CreatedAt  *time.Time  `json:"created_at,omitempty"`
}

type revisionNoteJSON struct {
	Milestone int       `json:"milestone"`
	Reason    string    `json:"reason"`
	TxHash    string    `json:"tx_hash"`
	CreatedAt time.Time `json:"created_at"`
}

type escrowJSON struct {
	ID          string          `json:"id"`
	Link        string          `json:"link"`
	ContractID  string          `json:"contract_id"`
	Status      string          `json:"status"`
	CreatorRole string          `json:"creator_role"`
	FeeBPS      uint16          `json:"fee_bps"`
	Total       string          `json:"total"`
	Milestones  []milestoneJSON `json:"milestones"`
	// Freelancer and Client are the parties' details: from the active
	// submission's terms once the other party submitted, otherwise the
	// creator's own.
	Freelancer      *freelancerJSON     `json:"freelancer"`
	Client          *clientJSON         `json:"client"`
	Submission      *submissionJSON     `json:"submission"`
	ReviewWindowSec *uint64             `json:"review_window_sec"`
	Balance         string              `json:"balance,omitempty"`
	PendingPayouts  []pendingPayoutJSON `json:"pending_payouts"`
	CreatedAtMS     *uint64             `json:"created_at_ms"`
	FundedAtMS      *uint64             `json:"funded_at_ms"`
	CreatedAt       time.Time           `json:"created_at"`
	UpdatedAt       time.Time           `json:"updated_at"`
}

// invitationJSON is the invitation as the creator sees it: its kind and
// the one value it names.
type invitationJSON struct {
	Kind     string      `json:"kind"`
	Username string      `json:"username,omitempty"`
	Email    string      `json:"email,omitempty"`
	Wallet   *walletJSON `json:"wallet,omitempty"`
}

func newInvitationJSON(inv *domain.Invitation) *invitationJSON {
	if inv == nil {
		return nil
	}
	out := &invitationJSON{Kind: string(inv.Kind), Username: inv.Username, Email: inv.Email}
	if inv.Kind == domain.InviteByWallet {
		out.Wallet = &walletJSON{Scheme: string(inv.Wallet.Scheme), Address: inv.Wallet.Address}
	}
	return out
}

// creatorEscrowJSON is an escrow with its invitation, for responses that
// only its creator receives.
type creatorEscrowJSON struct {
	escrowJSON
	Invitation *invitationJSON `json:"invitation"`
}

// viewerJSON tells the requester how their account relates to the escrow.
type viewerJSON struct {
	Creator bool `json:"creator"`
	Invited bool `json:"invited"`
}

type warningJSON struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type escrowDetailJSON struct {
	escrowJSON
	Transactions  []transactionJSON  `json:"transactions"`
	RevisionNotes []revisionNoteJSON `json:"revision_notes"`
	Viewer        viewerJSON         `json:"viewer"`
	// Invitation is shown to the creator only.
	Invitation *invitationJSON `json:"invitation"`
	Warnings   []warningJSON   `json:"warnings"`
}

func (h *EscrowHandler) escrowJSON(e domain.Escrow) escrowJSON {
	out := escrowJSON{
		ID:              e.ID,
		Link:            h.escrows.Link(e.ID),
		ContractID:      h.escrows.ContractID(),
		Status:          string(e.Status),
		CreatorRole:     string(e.CreatorRole),
		FeeBPS:          e.FeeBPS,
		Milestones:      []milestoneJSON{},
		ReviewWindowSec: e.ReviewWindowSec,
		Balance:         e.Balance,
		PendingPayouts:  []pendingPayoutJSON{},
		CreatedAtMS:     e.ChainCreatedAtMS,
		FundedAtMS:      e.FundedAtMS,
		CreatedAt:       e.CreatedAt,
		UpdatedAt:       e.UpdatedAt,
	}
	if total, err := domain.ValidateMilestones(e.MilestoneAmounts()); err == nil {
		out.Total = total.String()
	}
	for i, m := range e.Milestones {
		mj := milestoneJSON{Index: i, Amount: m.Amount, Status: string(m.Status), SubmittedAtMS: m.SubmittedAtMS, RevisionCount: m.RevisionCount}
		if m.Status == domain.MilestoneSubmitted && m.SubmittedAtMS != nil && e.ReviewWindowSec != nil {
			at := *m.SubmittedAtMS + *e.ReviewWindowSec*1000
			mj.AutoReleaseAtMS = &at
		}
		out.Milestones = append(out.Milestones, mj)
	}
	f, c := e.Freelancer, e.Client
	if s := e.Submission; s != nil {
		fd, cd := s.Terms.FreelancerDetails(), s.Terms.ClientDetails()
		f, c = &fd, &cd
		nonce := domain.TermsNonce(s.Message)
		out.Submission = &submissionJSON{
			ID:        s.ID,
			Status:    string(s.Status),
			Terms:     s.Terms.JSON(),
			Message:   s.Message,
			Nonce:     hex.EncodeToString(nonce[:]),
			Recipient: h.escrows.ContractID(),
			Wallet:    walletJSON{Scheme: string(s.Wallet.Scheme), Address: s.Wallet.Address},
			CreatedAt: s.CreatedAt,
		}
	}
	if f != nil {
		out.Freelancer = &freelancerJSON{Account: f.Account, Split: domain.SplitJSON(f.Split)}
	}
	if c != nil {
		out.Client = &clientJSON{Approver: c.Approver.JSON(), RefundAccount: c.RefundAccount}
	}
	for _, p := range e.PendingPayouts {
		out.PendingPayouts = append(out.PendingPayouts, pendingPayoutJSON{AccountID: p.AccountID, Amount: p.Amount})
	}
	return out
}

func (h *EscrowHandler) transactionJSON(tx domain.Transaction) transactionJSON {
	out := transactionJSON{
		Hash: tx.Hash, Method: tx.Method, SignerID: tx.SignerID, SenderID: tx.SenderID, ReceiverID: tx.ReceiverID,
		Succeeded: tx.Succeeded, Error: tx.Error, Events: []eventJSON{},
	}
	if h.explorerTxURL != "" && tx.Hash != "" {
		out.URL = h.explorerTxURL + tx.Hash
	}
	if !tx.CreatedAt.IsZero() {
		created := tx.CreatedAt
		out.CreatedAt = &created
	}
	for _, ev := range tx.Events {
		out.Events = append(out.Events, eventJSON{Event: ev.Event, Data: json.RawMessage(ev.Data)})
	}
	return out
}

type actionResponse struct {
	Escrow      escrowJSON       `json:"escrow"`
	Transaction *transactionJSON `json:"transaction"`
}

func (h *EscrowHandler) actionResponse(res escrow.ActionResult) actionResponse {
	out := actionResponse{Escrow: h.escrowJSON(res.Escrow)}
	if res.Transaction.Hash != "" {
		tx := h.transactionJSON(res.Transaction)
		out.Transaction = &tx
	}
	return out
}

type guestTokenResponse struct {
	GuestToken string    `json:"guest_token"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// --- Handlers ---------------------------------------------------------------

// Create stores a new Open escrow for the signed-in creator (R-23).
func (h *EscrowHandler) Create(c *gin.Context) {
	id, ok := accountID(c)
	if !ok {
		return
	}
	var req createEscrowRequest
	if !bindJSON(c, &req) {
		return
	}
	e, err := h.escrows.Create(c.Request.Context(), escrow.CreateInput{
		AccountID: id, Role: req.Role, Milestones: req.Milestones, Details: req.input(),
		Invitation: req.Invitation.input(),
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, creatorEscrowJSON{escrowJSON: h.escrowJSON(e), Invitation: newInvitationJSON(e.Invitation)})
}

// UpdateInvitation replaces or removes the invitation until the escrow is
// on-chain; creator only (R-15).
func (h *EscrowHandler) UpdateInvitation(c *gin.Context) {
	id, ok := accountID(c)
	if !ok {
		return
	}
	var req updateInvitationRequest
	if !bindJSON(c, &req) {
		return
	}
	if len(req.Invitation) == 0 {
		response.Error(c, domain.InvalidArgument("invitation is required; send null to remove it"))
		return
	}
	var inv *invitationRequest
	if err := json.Unmarshal(req.Invitation, &inv); err != nil {
		response.Error(c, domain.InvalidArgument("invitation must be an object with username, email or wallet, or null"))
		return
	}
	e, err := h.escrows.UpdateInvitation(c.Request.Context(), id, c.Param("id"), inv.input())
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, creatorEscrowJSON{escrowJSON: h.escrowJSON(e), Invitation: newInvitationJSON(e.Invitation)})
}

// Get shows an escrow to anyone with the link.
func (h *EscrowHandler) Get(c *gin.Context) {
	view, err := h.escrows.Get(c.Request.Context(), c.Param("id"), middleware.RequesterFrom(c.Request.Context()))
	if err != nil {
		response.Error(c, err)
		return
	}
	out := escrowDetailJSON{
		escrowJSON: h.escrowJSON(view.Escrow), Transactions: []transactionJSON{}, RevisionNotes: []revisionNoteJSON{},
		Viewer:     viewerJSON{Creator: view.Creator, Invited: view.Invited},
		Invitation: newInvitationJSON(view.Invitation), Warnings: []warningJSON{},
	}
	for _, w := range view.Warnings {
		out.Warnings = append(out.Warnings, warningJSON{Code: w.Code, Message: w.Message})
	}
	for _, tx := range view.Transactions {
		out.Transactions = append(out.Transactions, h.transactionJSON(tx))
	}
	for _, n := range view.RevisionNotes {
		out.RevisionNotes = append(out.RevisionNotes, revisionNoteJSON{Milestone: n.Milestone, Reason: n.Reason, TxHash: n.TxHash, CreatedAt: n.CreatedAt})
	}
	c.JSON(http.StatusOK, out)
}

type termsResponse struct {
	Terms     domain.TermsJSON `json:"terms"`
	Message   string           `json:"message"`
	Nonce     string           `json:"nonce"`
	Recipient string           `json:"recipient"`
}

// Terms builds the final terms and the exact message to sign from the other
// party's details; it changes nothing.
func (h *EscrowHandler) Terms(c *gin.Context) {
	var req detailsRequest
	if !bindJSON(c, &req) {
		return
	}
	t, err := h.escrows.BuildTerms(c.Request.Context(), c.Param("id"), req.input())
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, termsResponse{Terms: t.Terms.JSON(), Message: t.Message, Nonce: t.Nonce, Recipient: t.Recipient})
}

type submitResponse struct {
	Escrow escrowJSON `json:"escrow"`
	guestTokenResponse
}

// Counterparty stores the other party's signed submission and returns a
// guest token for the submitting wallet (R-23, R-21).
func (h *EscrowHandler) Counterparty(c *gin.Context) {
	var req submitRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := h.escrows.SubmitCounterparty(c.Request.Context(), c.Param("id"), escrow.SubmitInput{
		Details: req.input(), DeadlineMS: req.DeadlineMS, Signature: req.Signature, PublicKey: req.PublicKey,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, submitResponse{
		Escrow:             h.escrowJSON(res.Escrow),
		guestTokenResponse: guestTokenResponse{GuestToken: res.GuestToken, ExpiresAt: res.GuestExpiresAt},
	})
}

// Approve records the creator's signature over the submitted terms.
func (h *EscrowHandler) Approve(c *gin.Context) {
	id, ok := accountID(c)
	if !ok {
		return
	}
	var req approveRequest
	if !bindJSON(c, &req) {
		return
	}
	e, err := h.escrows.Approve(c.Request.Context(), id, c.Param("id"), escrow.ApproveInput{Signature: req.Signature, PublicKey: req.PublicKey})
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, h.escrowJSON(e))
}

// Reject discards the submission; the escrow returns to Open.
func (h *EscrowHandler) Reject(c *gin.Context) {
	id, ok := accountID(c)
	if !ok {
		return
	}
	e, err := h.escrows.Reject(c.Request.Context(), id, c.Param("id"))
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, h.escrowJSON(e))
}

// Cancel cancels before funding: by the creator's session while the escrow
// is off-chain, then by the freelancer's delegate action or the approver's
// signature.
func (h *EscrowHandler) Cancel(c *gin.Context) {
	var req cancelRequest
	if !bindOptionalJSON(c, &req) {
		return
	}
	in := escrow.CancelInput{AccountID: middleware.RequesterFrom(c.Request.Context()).AccountID}
	if req.DelegateAction != "" {
		raw, err := escrow.DecodeDelegate(req.DelegateAction)
		if err != nil {
			response.Error(c, err)
			return
		}
		in.Delegate = raw
	}
	if req.Signature != "" || req.Nonce != "" {
		a := approverActionRequest{Nonce: req.Nonce, DeadlineMS: req.DeadlineMS, Signature: req.Signature}.action()
		in.Approver = &a
	}
	res, err := h.escrows.Cancel(c.Request.Context(), c.Param("id"), in)
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, h.actionResponse(res))
}

// GuestToken renews a guest token with a signed login nonce from the wallet
// that submitted (R-21).
func (h *EscrowHandler) GuestToken(c *gin.Context) {
	var req proofRequest
	if !bindJSON(c, &req) {
		return
	}
	w, err := h.wallets.Prove(c.Request.Context(), req.proof())
	if err != nil {
		response.Error(c, err)
		return
	}
	token, exp, err := h.escrows.RenewGuestToken(c.Request.Context(), c.Param("id"), w)
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, guestTokenResponse{GuestToken: token, ExpiresAt: exp})
}

// milestone parses the :n path parameter.
func milestone(c *gin.Context) (uint32, bool) {
	n, err := strconv.ParseUint(c.Param("n"), 10, 32)
	if err != nil || n >= domain.MaxMilestones {
		response.Error(c, domain.NotFound("milestone not found"))
		return 0, false
	}
	return uint32(n), true
}

func (h *EscrowHandler) delegate(c *gin.Context) ([]byte, bool) {
	var req delegateRequest
	if !bindJSON(c, &req) {
		return nil, false
	}
	raw, err := escrow.DecodeDelegate(req.DelegateAction)
	if err != nil {
		response.Error(c, err)
		return nil, false
	}
	return raw, true
}

func (h *EscrowHandler) writeAction(c *gin.Context, res escrow.ActionResult, err error) {
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, h.actionResponse(res))
}

// SubmitMilestone relays the freelancer's submit_milestone (R-05, R-09).
func (h *EscrowHandler) SubmitMilestone(c *gin.Context) {
	n, ok := milestone(c)
	if !ok {
		return
	}
	raw, ok := h.delegate(c)
	if !ok {
		return
	}
	res, err := h.escrows.SubmitMilestone(c.Request.Context(), c.Param("id"), n, raw)
	h.writeAction(c, res, err)
}

// ApproveMilestone forwards the approver's signed approval (R-06, R-09).
func (h *EscrowHandler) ApproveMilestone(c *gin.Context) {
	n, ok := milestone(c)
	if !ok {
		return
	}
	var req approverActionRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := h.escrows.ApproveMilestone(c.Request.Context(), c.Param("id"), n, req.action())
	h.writeAction(c, res, err)
}

// RequestRevision forwards the approver's signed revision request and keeps
// its reason (R-17).
func (h *EscrowHandler) RequestRevision(c *gin.Context) {
	n, ok := milestone(c)
	if !ok {
		return
	}
	var req revisionRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := h.escrows.RequestRevision(c.Request.Context(), c.Param("id"), n, req.action(), req.Reason)
	h.writeAction(c, res, err)
}

// Refund relays the freelancer's refund_unsubmitted (R-18, R-09).
func (h *EscrowHandler) Refund(c *gin.Context) {
	raw, ok := h.delegate(c)
	if !ok {
		return
	}
	res, err := h.escrows.RefundUnsubmitted(c.Request.Context(), c.Param("id"), raw)
	h.writeAction(c, res, err)
}

type myEscrowJSON struct {
	escrowJSON
	Roles []string `json:"roles"`
	// Invited is true when the account was invited to the escrow (R-15);
	// the invited role is among Roles.
	Invited bool `json:"invited"`
}

type myEscrowsResponse struct {
	Escrows []myEscrowJSON `json:"escrows"`
}

// ListMine lists the signed-in account's escrows (R-16).
func (h *EscrowHandler) ListMine(c *gin.Context) {
	id, ok := accountID(c)
	if !ok {
		return
	}
	list, err := h.escrows.ListMine(c.Request.Context(), id, c.Query("role"), c.Query("status"))
	if err != nil {
		response.Error(c, err)
		return
	}
	out := myEscrowsResponse{Escrows: []myEscrowJSON{}}
	for _, ae := range list {
		roles := make([]string, len(ae.Roles))
		for i, r := range ae.Roles {
			roles[i] = string(r)
		}
		out.Escrows = append(out.Escrows, myEscrowJSON{escrowJSON: h.escrowJSON(ae.Escrow), Roles: roles, Invited: ae.Invited})
	}
	c.JSON(http.StatusOK, out)
}
