// Package funding implements cross-chain funding through 1Click (spec.md
// R-08): the token list, quotes for the approver, and watching the funding
// status until the swap ends.
package funding

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/harundarat/vecrow/backend/internal/domain"
)

// ErrNotFound is returned by Repository for an unknown quote.
var ErrNotFound = errors.New("funding: not found")

// ErrDepositNotFound is returned by OneClick.Status when 1Click does not
// know the deposit address (yet).
var ErrDepositNotFound = errors.New("funding: deposit address not found")

// QuoteRejectedError is a quote 1Click refused, with its reason.
type QuoteRejectedError struct{ Message string }

func (e *QuoteRejectedError) Error() string { return "1Click refused the quote: " + e.Message }

// Escrows is the escrow use case.
type Escrows interface {
	ContractID() string
	// Escrow returns the stored escrow.
	Escrow(ctx context.Context, id string) (domain.Escrow, error)
	// Sync refreshes the escrow from the contract.
	Sync(ctx context.Context, id string) error
}

// WalletOwners tells whether a wallet is linked to an account.
type WalletOwners interface {
	IsWalletOwner(ctx context.Context, accountID string, w domain.Wallet) (bool, error)
}

// Repository stores quotes and their funding status.
type Repository interface {
	InsertQuote(ctx context.Context, q domain.FundingQuote) error
	GetQuote(ctx context.Context, id string) (domain.FundingQuote, error)
	// LatestQuote returns the escrow's newest quote, or ErrNotFound.
	LatestQuote(ctx context.Context, escrowID string) (domain.FundingQuote, error)
	// UpdateStatus stores a status read from 1Click and when it was read.
	UpdateStatus(ctx context.Context, id string, status domain.FundingStatus, oneClickStatus string, watching bool) error
	// ListWatching returns quotes still watched and last checked before
	// checkedBefore (or never), oldest check first.
	ListWatching(ctx context.Context, checkedBefore time.Time, limit int) ([]string, error)
}

// QuoteRequest is an EXACT_OUTPUT quote request.
type QuoteRequest struct {
	OriginAsset      string
	DestinationAsset string
	// Amount is the exact output, in base units of DestinationAsset.
	Amount             string
	RefundTo           string
	Recipient          string
	CustomRecipientMsg string
	SlippageBPS        int
	Deadline           time.Time
}

// Quote is 1Click's answer to a quote request.
type Quote struct {
	DepositAddress    string
	DepositMemo       string
	AmountIn          string
	AmountInFormatted string
	AmountOut         string
	Deadline          *time.Time
	// Raw is the response as 1Click returned it.
	Raw []byte
}

// OneClick is the 1Click API.
type OneClick interface {
	Tokens(ctx context.Context) ([]domain.Token, error)
	// Quote returns *QuoteRejectedError when 1Click refuses the request.
	Quote(ctx context.Context, r QuoteRequest) (Quote, error)
	// Status returns the raw swap status, or ErrDepositNotFound.
	Status(ctx context.Context, depositAddress, depositMemo string) (string, error)
}

// Config holds the funding settings.
type Config struct {
	// USDCAssetID is the 1Click asset ID of the USDC the escrow holds,
	// "nep141:<USDC contract>".
	USDCAssetID string
	// SlippageBPS is the slippage tolerance of quotes.
	SlippageBPS int
	// QuoteTTL is how long a quote's deposit address is valid.
	QuoteTTL time.Duration
	// WatchGrace is how long after a quote's deadline its status is still
	// watched, for deposits 1Click processes late.
	WatchGrace time.Duration
	// RefreshAfter is the age of a stored status after which GET funding
	// reads 1Click again.
	RefreshAfter time.Duration
	// TokensTTL is how long the token list is cached.
	TokensTTL time.Duration
}

// Service implements the funding use cases.
type Service struct {
	escrows  Escrows
	wallets  WalletOwners
	repo     Repository
	oneclick OneClick
	cfg      Config
	now      func() time.Time
	log      *slog.Logger

	mu       sync.Mutex
	tokens   []domain.Token
	tokensAt time.Time
}

// Option configures the service.
type Option func(*Service)

func WithClock(now func() time.Time) Option { return func(s *Service) { s.now = now } }

func WithLogger(l *slog.Logger) Option { return func(s *Service) { s.log = l } }

func NewService(escrows Escrows, wallets WalletOwners, repo Repository, oc OneClick, cfg Config, opts ...Option) *Service {
	s := &Service{escrows: escrows, wallets: wallets, repo: repo, oneclick: oc, cfg: cfg,
		now: time.Now, log: slog.New(slog.DiscardHandler)}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Tokens returns the tokens 1Click can swap from, cached for TokensTTL.
func (s *Service) Tokens(ctx context.Context) ([]domain.Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tokens != nil && s.now().Sub(s.tokensAt) < s.cfg.TokensTTL {
		return s.tokens, nil
	}
	tokens, err := s.oneclick.Tokens(ctx)
	if err != nil {
		if s.tokens != nil {
			s.log.WarnContext(ctx, "1Click token list refresh failed, serving the cached list", slog.String("error", err.Error()))
			return s.tokens, nil
		}
		return nil, fmt.Errorf("%w (%w)", domain.Unavailable("the 1Click token list is unavailable"), err)
	}
	s.tokens, s.tokensAt = tokens, s.now()
	return tokens, nil
}

// requireApprover checks that the requester proves the approver wallet of
// the escrow: with a guest token for this escrow and that wallet, or with a
// session whose account owns it (R-08, R-21).
func (s *Service) requireApprover(ctx context.Context, e domain.Escrow, req domain.Requester) error {
	terms, ok := e.Terms()
	if !ok {
		return domain.Conflict("the escrow is %s; funding starts once it is Created", e.Status)
	}
	if req.Guest == nil && req.AccountID == "" {
		return domain.Unauthorized("a guest token or a session for the approver wallet is required")
	}
	approver := terms.Approver.Wallet()
	if g := req.Guest; g != nil {
		if g.EscrowID != e.ID {
			return domain.Unauthorized("guest token is for another escrow")
		}
		if g.Wallet == approver {
			return nil
		}
	}
	if req.AccountID != "" {
		owns, err := s.wallets.IsWalletOwner(ctx, req.AccountID, approver)
		if err != nil {
			return fmt.Errorf("funding: check approver wallet: %w", err)
		}
		if owns {
			return nil
		}
	}
	return domain.Forbidden("only the approver wallet can fund this escrow")
}

// QuoteInput is the client's choice of source token and refund address.
type QuoteInput struct {
	OriginAsset string
	// RefundTo receives the deposit back on the origin chain if the swap
	// fails.
	RefundTo string
}

// RequestQuote asks 1Click for a deposit address that delivers exactly the
// escrow's total in USDC to the contract, with the escrow ID as the funding
// message (R-08). Only the approver can, and only while the escrow is
// Created.
func (s *Service) RequestQuote(ctx context.Context, id string, req domain.Requester, in QuoteInput) (domain.FundingQuote, error) {
	in.OriginAsset, in.RefundTo = strings.TrimSpace(in.OriginAsset), strings.TrimSpace(in.RefundTo)
	if in.OriginAsset == "" || in.RefundTo == "" {
		return domain.FundingQuote{}, domain.InvalidArgument("origin_asset and refund_to are required")
	}
	e, err := s.escrows.Escrow(ctx, id)
	if err != nil {
		return domain.FundingQuote{}, err
	}
	if err := s.requireApprover(ctx, e, req); err != nil {
		return domain.FundingQuote{}, err
	}
	// The stored status may lag behind the contract.
	if err := s.escrows.Sync(ctx, id); err != nil {
		return domain.FundingQuote{}, fmt.Errorf("%w (%w)", domain.Unavailable("the escrow could not be read from the contract"), err)
	}
	if e, err = s.escrows.Escrow(ctx, id); err != nil {
		return domain.FundingQuote{}, err
	}
	if e.Status != domain.EscrowCreated {
		return domain.FundingQuote{}, domain.Conflict("quotes are only available while the escrow is Created; it is %s", e.Status)
	}
	terms, _ := e.Terms()
	total := terms.Total().String()

	deadline := s.now().Add(s.cfg.QuoteTTL).UTC().Truncate(time.Second)
	q, err := s.oneclick.Quote(ctx, QuoteRequest{
		OriginAsset:        in.OriginAsset,
		DestinationAsset:   s.cfg.USDCAssetID,
		Amount:             total,
		RefundTo:           in.RefundTo,
		Recipient:          s.escrows.ContractID(),
		CustomRecipientMsg: id,
		SlippageBPS:        s.cfg.SlippageBPS,
		Deadline:           deadline,
	})
	var rejected *QuoteRejectedError
	if errors.As(err, &rejected) {
		return domain.FundingQuote{}, domain.InvalidArgument("1Click refused the quote: %s", rejected.Message)
	}
	if err != nil {
		return domain.FundingQuote{}, fmt.Errorf("%w (%w)", domain.Unavailable("1Click is unavailable"), err)
	}
	if q.DepositAddress == "" {
		return domain.FundingQuote{}, fmt.Errorf("%w (no deposit address)", domain.Unavailable("1Click returned no deposit address"))
	}
	if q.AmountOut != total {
		// EXACT_OUTPUT must deliver the total; anything else would not
		// fund the escrow (R-02).
		return domain.FundingQuote{}, fmt.Errorf("%w (amountOut %s, want %s)",
			domain.Unavailable("1Click quoted another amount"), q.AmountOut, total)
	}
	qid, err := uuid.NewV7()
	if err != nil {
		return domain.FundingQuote{}, fmt.Errorf("funding: new id: %w", err)
	}
	if q.Deadline == nil {
		q.Deadline = &deadline
	}
	fq := domain.FundingQuote{
		ID:                qid.String(),
		EscrowID:          id,
		OriginAsset:       in.OriginAsset,
		RefundTo:          in.RefundTo,
		DepositAddress:    q.DepositAddress,
		DepositMemo:       q.DepositMemo,
		AmountIn:          q.AmountIn,
		AmountInFormatted: q.AmountInFormatted,
		AmountOut:         q.AmountOut,
		Deadline:          q.Deadline,
		Status:            domain.FundingPending,
		Watching:          true,
		Raw:               q.Raw,
		CreatedAt:         s.now(),
	}
	if err := s.repo.InsertQuote(ctx, fq); err != nil {
		return domain.FundingQuote{}, fmt.Errorf("funding: store quote: %w", err)
	}
	s.log.InfoContext(ctx, "funding quote created",
		slog.String("escrow_id", id), slog.String("quote_id", fq.ID), slog.String("origin_asset", in.OriginAsset))
	return fq, nil
}

// View is the funding status of an escrow.
type View struct {
	// Status is SUCCESS once the escrow holds its funds on-chain, whichever
	// way they arrived; otherwise the latest quote's status.
	Status       domain.FundingStatus
	Quote        domain.FundingQuote
	EscrowStatus domain.EscrowStatus
}

// Status returns the funding status of the escrow's latest quote, read
// again from 1Click when the stored one is older than RefreshAfter.
func (s *Service) Status(ctx context.Context, id string, req domain.Requester) (View, error) {
	e, err := s.escrows.Escrow(ctx, id)
	if err != nil {
		return View{}, err
	}
	if err := s.requireApprover(ctx, e, req); err != nil {
		return View{}, err
	}
	q, err := s.repo.LatestQuote(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return View{}, domain.NotFound("no quote has been requested for this escrow")
	}
	if err != nil {
		return View{}, fmt.Errorf("funding: latest quote: %w", err)
	}
	if q.Watching && (q.CheckedAt == nil || s.now().Sub(*q.CheckedAt) >= s.cfg.RefreshAfter) {
		if err := s.Check(ctx, q.ID); err != nil {
			s.log.WarnContext(ctx, "funding status refresh failed, showing the stored status",
				slog.String("escrow_id", id), slog.String("error", err.Error()))
		} else if q, err = s.repo.GetQuote(ctx, q.ID); err != nil {
			return View{}, fmt.Errorf("funding: get quote: %w", err)
		}
	}
	if e, err = s.escrows.Escrow(ctx, id); err != nil {
		return View{}, err
	}
	v := View{Status: q.Status, Quote: q, EscrowStatus: e.Status}
	if e.Status == domain.EscrowFunded || e.Status == domain.EscrowCompleted {
		v.Status = domain.FundingSuccess
	}
	return v, nil
}

// fundingStatus maps a 1Click status to the status shown to the client.
// FAILED is not final for the client, who can request a new quote.
func fundingStatus(oneClick string) domain.FundingStatus {
	switch oneClick {
	case "SUCCESS":
		return domain.FundingSuccess
	case "REFUNDED":
		return domain.FundingRefunded
	default:
		return domain.FundingPending
	}
}

// terminal reports whether 1Click will not change the status again.
func terminal(oneClick string) bool {
	return oneClick == "SUCCESS" || oneClick == "REFUNDED" || oneClick == "FAILED"
}

// Check reads the quote's status from 1Click and stores it (the funding
// watcher job). It stops watching at a final status, or WatchGrace after
// the deadline. A successful swap refreshes the escrow from the contract.
func (s *Service) Check(ctx context.Context, quoteID string) error {
	q, err := s.repo.GetQuote(ctx, quoteID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("funding: get quote: %w", err)
	}
	if !q.Watching {
		return nil
	}
	raw, err := s.oneclick.Status(ctx, q.DepositAddress, q.DepositMemo)
	if errors.Is(err, ErrDepositNotFound) {
		raw, err = q.OneClickStatus, nil // nothing deposited that 1Click knows of
	}
	if err != nil {
		return fmt.Errorf("funding: 1Click status: %w", err)
	}
	expired := q.Deadline != nil && s.now().After(q.Deadline.Add(s.cfg.WatchGrace))
	watching := !terminal(raw) && !expired
	status := fundingStatus(raw)
	if err := s.repo.UpdateStatus(ctx, q.ID, status, raw, watching); err != nil {
		return fmt.Errorf("funding: store status: %w", err)
	}
	if raw != q.OneClickStatus {
		s.log.InfoContext(ctx, "funding status changed", slog.String("escrow_id", q.EscrowID),
			slog.String("quote_id", q.ID), slog.String("oneclick_status", raw))
	}
	if status == domain.FundingSuccess {
		if err := s.escrows.Sync(ctx, q.EscrowID); err != nil {
			s.log.WarnContext(ctx, "escrow refresh after funding failed",
				slog.String("escrow_id", q.EscrowID), slog.String("error", err.Error()))
		}
	}
	return nil
}

// TopicCheck is the queue topic of funding watcher jobs.
const TopicCheck = "funding.check"

// CheckJob is the payload of a TopicCheck message.
type CheckJob struct {
	QuoteID string `json:"quote_id"`
}

// FindDue returns the quotes to check: watched and last checked at least
// interval ago.
func (s *Service) FindDue(ctx context.Context, interval time.Duration, limit int) ([]string, error) {
	ids, err := s.repo.ListWatching(ctx, s.now().Add(-interval), limit)
	if err != nil {
		return nil, fmt.Errorf("funding: list watched quotes: %w", err)
	}
	return ids, nil
}
