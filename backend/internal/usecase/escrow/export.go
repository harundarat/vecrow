package escrow

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"time"

	"github.com/harundarat/vecrow/backend/internal/domain"
)

// Payout statuses in the CSV export (R-12).
const (
	PayoutPaid = "paid"
	// PayoutPending is an amount the contract holds for the recipient
	// because the transfer failed (R-20).
	PayoutPending = "pending"
)

// Payout is one payout of an escrow, a row of the CSV export (R-12): the
// platform fee to the treasury, a share of a released milestone to a split
// recipient, or a refund to the client.
type Payout struct {
	// Time is when the transaction that paid it, or that left it pending,
	// was stored. A pending amount no stored event explains has the time of
	// the last snapshot.
	Time     time.Time
	EscrowID string
	// Milestones are the 0-based milestones the payout is for: one for a
	// release, all refunded ones for a refund. Empty when no stored event
	// names them.
	Milestones []uint32
	// Amount is in USDC base units.
	Amount    *big.Int
	Recipient string
	// TxHash is the transaction that paid it, or that left it pending;
	// empty when it is not in the index.
	TxHash string
	Status string
}

// The NEP-297 events the payouts are built from (architecture.md §5.5).
const (
	eventMilestoneReleased  = "milestone_released"
	eventMilestonesRefunded = "milestones_refunded"
	eventPayout             = "payout"
)

// Payouts returns the payouts of an escrow for the CSV export (R-12), from
// the stored transactions, their events and the last snapshot of the
// pending payouts; it does not read the contract. Only a party can: the
// creator, an account with a linked party wallet, or a guest token for this
// escrow whose wallet is a party.
func (s *Service) Payouts(ctx context.Context, id string, req domain.Requester) ([]Payout, error) {
	if req.Guest != nil && req.Guest.EscrowID != id {
		return nil, domain.Unauthorized("guest token is for another escrow")
	}
	e, err := s.get(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.requireParty(ctx, e, req); err != nil {
		return nil, err
	}
	txs, err := s.Repo.ListTransactions(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("escrow: list transactions: %w", err)
	}
	return payouts(e, txs)
}

// requireParty checks that the requester is a party of the escrow.
func (s *Service) requireParty(ctx context.Context, e domain.Escrow, req domain.Requester) error {
	if req.Guest == nil && req.AccountID == "" {
		return domain.Unauthorized("a session or a guest token for this escrow is required")
	}
	parties := e.Parties()
	if g := req.Guest; g != nil {
		for _, p := range parties {
			if p.Wallet == g.Wallet {
				return nil
			}
		}
	}
	if req.AccountID != "" {
		if req.AccountID == e.CreatorAccountID {
			return nil
		}
		for _, p := range parties {
			owns, err := s.Wallets.IsWalletOwner(ctx, req.AccountID, p.Wallet)
			if err != nil {
				return fmt.Errorf("escrow: check party wallet: %w", err)
			}
			if owns {
				return nil
			}
		}
	}
	return domain.Forbidden("only the parties of the escrow can export its payouts")
}

// payouts builds one row per payout from the transactions, in the order
// they were sent and their events in execution order.
//
// Each payout event of a release or refund is a row, paid or pending. A
// retry_payout pays the recipient's whole pending amount, so a paid retry
// marks every pending row of that recipient paid, with the retry's time and
// hash; any rest of the retried amount is a row of its own. Pending amounts
// in the snapshot beyond what the pending rows explain are rows too, so
// every pending payout appears. Pending rows are never marked paid without
// a stored transaction that paid them.
func payouts(e domain.Escrow, txs []domain.Transaction) ([]Payout, error) {
	type row struct {
		Payout
		order int // position of the paying transaction, for sorting
	}
	var rows []row
	pending := map[string][]int{}
	for ti, tx := range txs {
		var milestones []uint32
		inRelease := false
		for _, ev := range tx.Events {
			switch ev.Event {
			case eventMilestoneReleased:
				var d struct {
					EscrowID  string `json:"escrow_id"`
					Milestone uint32 `json:"milestone"`
				}
				if err := json.Unmarshal(ev.Data, &d); err != nil {
					return nil, fmt.Errorf("escrow: %s event in %s: %w", ev.Event, tx.Hash, err)
				}
				if d.EscrowID == e.ID {
					milestones, inRelease = []uint32{d.Milestone}, true
				}
			case eventMilestonesRefunded:
				var d struct {
					EscrowID   string   `json:"escrow_id"`
					Milestones []uint32 `json:"milestones"`
				}
				if err := json.Unmarshal(ev.Data, &d); err != nil {
					return nil, fmt.Errorf("escrow: %s event in %s: %w", ev.Event, tx.Hash, err)
				}
				if d.EscrowID == e.ID {
					milestones, inRelease = d.Milestones, true
				}
			case eventPayout:
				var d struct {
					EscrowID  string `json:"escrow_id"`
					AccountID string `json:"account_id"`
					Amount    string `json:"amount"`
					Paid      bool   `json:"paid"`
				}
				if err := json.Unmarshal(ev.Data, &d); err != nil {
					return nil, fmt.Errorf("escrow: %s event in %s: %w", ev.Event, tx.Hash, err)
				}
				if d.EscrowID != e.ID {
					continue
				}
				amount, err := domain.ParseBaseUnits(d.Amount)
				if err != nil {
					return nil, fmt.Errorf("escrow: %s event in %s: %w", ev.Event, tx.Hash, err)
				}
				if inRelease {
					status := PayoutPaid
					if !d.Paid {
						status = PayoutPending
						pending[d.AccountID] = append(pending[d.AccountID], len(rows))
					}
					rows = append(rows, row{Payout{
						Time: tx.CreatedAt, EscrowID: e.ID, Milestones: milestones, Amount: amount,
						Recipient: d.AccountID, TxHash: tx.Hash, Status: status,
					}, ti})
					continue
				}
				// A retry_payout. If it failed, the amount stays pending.
				if !d.Paid {
					continue
				}
				rest := new(big.Int).Set(amount)
				for _, i := range pending[d.AccountID] {
					r := &rows[i]
					r.Status, r.Time, r.TxHash, r.order = PayoutPaid, tx.CreatedAt, tx.Hash, ti
					rest.Sub(rest, r.Amount)
				}
				delete(pending, d.AccountID)
				if rest.Sign() > 0 {
					rows = append(rows, row{Payout{
						Time: tx.CreatedAt, EscrowID: e.ID, Amount: rest,
						Recipient: d.AccountID, TxHash: tx.Hash, Status: PayoutPaid,
					}, ti})
				}
			}
		}
	}

	snapshot := append([]domain.PendingPayout(nil), e.PendingPayouts...)
	sort.Slice(snapshot, func(i, j int) bool { return snapshot[i].AccountID < snapshot[j].AccountID })
	for _, p := range snapshot {
		held, err := domain.ParseBaseUnits(p.Amount)
		if err != nil {
			return nil, fmt.Errorf("escrow: pending payout of %s: %w", p.AccountID, err)
		}
		for _, i := range pending[p.AccountID] {
			held.Sub(held, rows[i].Amount)
		}
		if held.Sign() > 0 {
			rows = append(rows, row{Payout{
				Time: e.UpdatedAt, EscrowID: e.ID, Amount: held,
				Recipient: p.AccountID, Status: PayoutPending,
			}, len(txs)})
		}
	}

	sort.SliceStable(rows, func(i, j int) bool { return rows[i].order < rows[j].order })
	out := make([]Payout, len(rows))
	for i, r := range rows {
		out[i] = r.Payout
	}
	return out, nil
}
