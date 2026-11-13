package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/harundarat/vecrow/backend/internal/domain"
	"github.com/harundarat/vecrow/backend/internal/repository/dbgen"
	"github.com/harundarat/vecrow/backend/internal/usecase/escrow"
)

// TxDB is a DB that also runs transactions; nested calls join the outer
// one. *database.Postgres implements it.
type TxDB interface {
	DB
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// maxAccountEscrows bounds the escrows listed for one account.
const maxAccountEscrows = 200

// EscrowRepository stores escrows in PostgreSQL.
type EscrowRepository struct {
	db TxDB
}

var _ escrow.Repository = (*EscrowRepository)(nil)

func NewEscrowRepository(db TxDB) *EscrowRepository {
	return &EscrowRepository{db: db}
}

func (r *EscrowRepository) q(ctx context.Context) *dbgen.Queries {
	return dbgen.New(r.db.Querier(ctx))
}

func (r *EscrowRepository) Create(ctx context.Context, e domain.Escrow) error {
	creator, err := uuid.Parse(e.CreatorAccountID)
	if err != nil {
		return fmt.Errorf("escrow repository: invalid creator account id: %w", err)
	}
	params := dbgen.InsertEscrowParams{
		ID:               e.ID,
		CreatorAccountID: creator,
		CreatorRole:      string(e.CreatorRole),
		Status:           string(e.Status),
		FeeBps:           int32(e.FeeBPS),
	}
	if f := e.Freelancer; f != nil {
		params.Freelancer = &f.Account
		if params.Split, err = json.Marshal(domain.SplitJSON(f.Split)); err != nil {
			return fmt.Errorf("escrow repository: encode split: %w", err)
		}
	}
	if c := e.Client; c != nil {
		params.RefundAccount = &c.RefundAccount
		if params.Approver, err = json.Marshal(c.Approver.JSON()); err != nil {
			return fmt.Errorf("escrow repository: encode approver: %w", err)
		}
	}
	return r.db.WithinTx(ctx, func(ctx context.Context) error {
		q := r.q(ctx)
		if err := q.InsertEscrow(ctx, params); err != nil {
			return fmt.Errorf("escrow repository: insert escrow: %w", err)
		}
		err := q.InsertMilestones(ctx, dbgen.InsertMilestonesParams{EscrowID: e.ID, Amounts: e.MilestoneAmounts()})
		if err != nil {
			return fmt.Errorf("escrow repository: insert milestones: %w", err)
		}
		return nil
	})
}

func (r *EscrowRepository) Get(ctx context.Context, id string) (domain.Escrow, error) {
	row, err := r.q(ctx).GetEscrow(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Escrow{}, escrow.ErrNotFound
	}
	if err != nil {
		return domain.Escrow{}, fmt.Errorf("escrow repository: get escrow: %w", err)
	}
	return r.load(ctx, row)
}

func (r *EscrowRepository) Lock(ctx context.Context, id string) (domain.Escrow, error) {
	row, err := r.q(ctx).LockEscrow(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Escrow{}, escrow.ErrNotFound
	}
	if err != nil {
		return domain.Escrow{}, fmt.Errorf("escrow repository: lock escrow: %w", err)
	}
	return r.load(ctx, row)
}

// load reads the rest of an escrow: milestones, active submission and
// pending payouts.
func (r *EscrowRepository) load(ctx context.Context, row dbgen.Escrow) (domain.Escrow, error) {
	e := domain.Escrow{
		ID:               row.ID,
		CreatorAccountID: row.CreatorAccountID.String(),
		CreatorRole:      domain.Role(row.CreatorRole),
		Status:           domain.EscrowStatus(row.Status),
		FeeBPS:           uint16(row.FeeBps),
		ReviewWindowSec:  toUint64(row.ReviewWindowSec),
		Balance:          deref(row.Balance),
		ChainCreatedAtMS: toUint64(row.ChainCreatedAtMs),
		FundedAtMS:       toUint64(row.FundedAtMs),
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
	}
	if row.Freelancer != nil {
		var split []domain.SplitShareJSON
		if err := json.Unmarshal(row.Split, &split); err != nil {
			return domain.Escrow{}, fmt.Errorf("escrow repository: decode split of %s: %w", row.ID, err)
		}
		e.Freelancer = &domain.FreelancerDetails{Account: *row.Freelancer, Split: domain.ParseSplitJSON(split)}
	}
	if row.Approver != nil {
		var aj domain.ApproverJSON
		if err := json.Unmarshal(row.Approver, &aj); err != nil {
			return domain.Escrow{}, fmt.Errorf("escrow repository: decode approver of %s: %w", row.ID, err)
		}
		approver, err := aj.Approver()
		if err != nil {
			return domain.Escrow{}, fmt.Errorf("escrow repository: approver of %s: %w", row.ID, err)
		}
		e.Client = &domain.ClientDetails{Approver: approver, RefundAccount: deref(row.RefundAccount)}
	}

	q := r.q(ctx)
	milestones, err := q.ListMilestones(ctx, row.ID)
	if err != nil {
		return domain.Escrow{}, fmt.Errorf("escrow repository: list milestones: %w", err)
	}
	for _, m := range milestones {
		e.Milestones = append(e.Milestones, domain.Milestone{
			Amount:        m.Amount,
			Status:        domain.MilestoneStatus(m.Status),
			SubmittedAtMS: toUint64(m.SubmittedAtMs),
			RevisionCount: int(m.RevisionCount),
		})
	}

	sub, err := q.GetActiveSubmission(ctx, row.ID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return domain.Escrow{}, fmt.Errorf("escrow repository: get submission: %w", err)
	default:
		s, err := toSubmission(sub)
		if err != nil {
			return domain.Escrow{}, err
		}
		e.Submission = &s
	}

	payouts, err := q.ListEscrowPendingPayouts(ctx, row.ID)
	if err != nil {
		return domain.Escrow{}, fmt.Errorf("escrow repository: list pending payouts: %w", err)
	}
	for _, p := range payouts {
		e.PendingPayouts = append(e.PendingPayouts, domain.PendingPayout{AccountID: p.AccountID, Amount: p.Amount})
	}

	inv, err := q.GetInvitation(ctx, row.ID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return domain.Escrow{}, fmt.Errorf("escrow repository: get invitation: %w", err)
	default:
		i := domain.Invitation{
			Kind: domain.InvitationKind(inv.Kind), Username: deref(inv.Username), Email: deref(inv.Email),
		}
		if inv.AccountID != nil {
			i.AccountID = inv.AccountID.String()
		}
		if inv.WalletScheme != nil {
			i.Wallet = domain.Wallet{Scheme: domain.WalletScheme(*inv.WalletScheme), Address: deref(inv.WalletAddress)}
		}
		e.Invitation = &i
	}
	return e, nil
}

// SetInvitation stores the escrow's invitation, replacing an earlier one;
// nil removes it.
func (r *EscrowRepository) SetInvitation(ctx context.Context, escrowID string, inv *domain.Invitation) error {
	q := r.q(ctx)
	if inv == nil {
		if err := q.DeleteInvitation(ctx, escrowID); err != nil {
			return fmt.Errorf("escrow repository: delete invitation: %w", err)
		}
		return nil
	}
	params := dbgen.UpsertInvitationParams{EscrowID: escrowID, Kind: string(inv.Kind), Email: nullable(inv.Email)}
	if inv.AccountID != "" {
		id, err := uuid.Parse(inv.AccountID)
		if err != nil {
			return fmt.Errorf("escrow repository: invited account %q: %w", inv.AccountID, err)
		}
		params.AccountID = &id
	}
	if inv.Wallet != (domain.Wallet{}) {
		params.WalletScheme, params.WalletAddress = nullable(string(inv.Wallet.Scheme)), nullable(inv.Wallet.Address)
	}
	if err := q.UpsertInvitation(ctx, params); err != nil {
		return fmt.Errorf("escrow repository: save invitation: %w", err)
	}
	return nil
}

func toSubmission(row dbgen.EscrowSubmission) (domain.Submission, error) {
	var tj domain.TermsJSON
	if err := json.Unmarshal(row.Terms, &tj); err != nil {
		return domain.Submission{}, fmt.Errorf("escrow repository: decode terms of submission %s: %w", row.ID, err)
	}
	terms, err := tj.Terms()
	if err != nil {
		return domain.Submission{}, fmt.Errorf("escrow repository: terms of submission %s: %w", row.ID, err)
	}
	return domain.Submission{
		ID:                    row.ID.String(),
		Status:                domain.SubmissionStatus(row.Status),
		Terms:                 terms,
		Message:               row.Message,
		Wallet:                domain.Wallet{Scheme: domain.WalletScheme(row.WalletScheme), Address: row.WalletAddress},
		CounterpartySignature: row.CounterpartySignature,
		CounterpartyPublicKey: deref(row.CounterpartyPublicKey),
		CreatorSignature:      deref(row.CreatorSignature),
		CreatorPublicKey:      deref(row.CreatorPublicKey),
		FailureReason:         deref(row.FailureReason),
		CreatedAt:             row.CreatedAt,
	}, nil
}

func (r *EscrowRepository) SetStatus(ctx context.Context, id string, status domain.EscrowStatus) error {
	n, err := r.q(ctx).UpdateEscrowStatus(ctx, dbgen.UpdateEscrowStatusParams{ID: id, Status: string(status)})
	if err != nil {
		return fmt.Errorf("escrow repository: update status: %w", err)
	}
	if n == 0 {
		return escrow.ErrNotFound
	}
	return nil
}

func (r *EscrowRepository) InsertSubmission(ctx context.Context, escrowID string, s domain.Submission) error {
	id, err := uuid.Parse(s.ID)
	if err != nil {
		return fmt.Errorf("escrow repository: invalid submission id: %w", err)
	}
	terms, err := json.Marshal(s.Terms.JSON())
	if err != nil {
		return fmt.Errorf("escrow repository: encode terms: %w", err)
	}
	// ON CONFLICT DO NOTHING returns no row while another submission is
	// active, without aborting the transaction.
	_, err = r.q(ctx).InsertSubmission(ctx, dbgen.InsertSubmissionParams{
		ID:                    id,
		EscrowID:              escrowID,
		Status:                string(s.Status),
		Terms:                 terms,
		Message:               s.Message,
		DeadlineMs:            int64(s.Terms.DeadlineMS),
		WalletScheme:          string(s.Wallet.Scheme),
		WalletAddress:         s.Wallet.Address,
		CounterpartySignature: s.CounterpartySignature,
		CounterpartyPublicKey: nullable(s.CounterpartyPublicKey),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return escrow.ErrActiveSubmission
	}
	if err != nil {
		return fmt.Errorf("escrow repository: insert submission: %w", err)
	}
	return nil
}

func (r *EscrowRepository) UpdateSubmission(ctx context.Context, s domain.Submission) error {
	id, err := uuid.Parse(s.ID)
	if err != nil {
		return escrow.ErrNotFound
	}
	n, err := r.q(ctx).UpdateSubmission(ctx, dbgen.UpdateSubmissionParams{
		ID:               id,
		Status:           string(s.Status),
		CreatorSignature: nullable(s.CreatorSignature),
		CreatorPublicKey: nullable(s.CreatorPublicKey),
		FailureReason:    nullable(s.FailureReason),
	})
	if err != nil {
		return fmt.Errorf("escrow repository: update submission: %w", err)
	}
	if n == 0 {
		return escrow.ErrNotFound
	}
	return nil
}

func (r *EscrowRepository) ReplaceParties(ctx context.Context, escrowID string, parties []domain.Party) error {
	params := dbgen.InsertPartiesParams{EscrowID: escrowID}
	for _, p := range parties {
		params.Roles = append(params.Roles, string(p.Role))
		params.Schemes = append(params.Schemes, string(p.Wallet.Scheme))
		params.Addresses = append(params.Addresses, p.Wallet.Address)
	}
	return r.db.WithinTx(ctx, func(ctx context.Context) error {
		q := r.q(ctx)
		if err := q.DeleteParties(ctx, escrowID); err != nil {
			return fmt.Errorf("escrow repository: delete parties: %w", err)
		}
		if len(parties) == 0 {
			return nil
		}
		if err := q.InsertParties(ctx, params); err != nil {
			return fmt.Errorf("escrow repository: insert parties: %w", err)
		}
		return nil
	})
}

func (r *EscrowRepository) SaveChain(ctx context.Context, c domain.ChainEscrow) (bool, error) {
	window, created, height := int64(c.ReviewWindowSec), int64(c.CreatedAtMS), int64(c.BlockHeight)
	balance := c.Balance
	milestones := dbgen.UpdateMilestonesChainParams{EscrowID: c.EscrowID}
	for i, m := range c.Milestones {
		submitted := int64(-1)
		if m.SubmittedAtMS != nil {
			submitted = int64(*m.SubmittedAtMS)
		}
		milestones.Milestones = append(milestones.Milestones, int32(i))
		milestones.Statuses = append(milestones.Statuses, string(m.Status))
		milestones.SubmittedAtMs = append(milestones.SubmittedAtMs, submitted)
		milestones.RevisionCounts = append(milestones.RevisionCounts, int32(m.RevisionCount))
	}
	payouts := dbgen.InsertPendingPayoutsParams{EscrowID: c.EscrowID}
	for _, p := range c.PendingPayouts {
		payouts.AccountIds = append(payouts.AccountIds, p.AccountID)
		payouts.Amounts = append(payouts.Amounts, p.Amount)
	}

	stored := false
	err := r.db.WithinTx(ctx, func(ctx context.Context) error {
		q := r.q(ctx)
		_, err := q.UpdateEscrowChain(ctx, dbgen.UpdateEscrowChainParams{
			ID:               c.EscrowID,
			Status:           string(c.Status),
			ReviewWindowSec:  &window,
			Balance:          &balance,
			ChainCreatedAtMs: &created,
			FundedAtMs:       toInt64(c.FundedAtMS),
			ChainBlockHeight: &height,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // a newer snapshot is stored
		}
		if err != nil {
			return fmt.Errorf("escrow repository: update chain state: %w", err)
		}
		if err := q.UpdateMilestonesChain(ctx, milestones); err != nil {
			return fmt.Errorf("escrow repository: update milestones: %w", err)
		}
		if err := q.DeletePendingPayouts(ctx, c.EscrowID); err != nil {
			return fmt.Errorf("escrow repository: delete pending payouts: %w", err)
		}
		if len(payouts.AccountIds) > 0 {
			if err := q.InsertPendingPayouts(ctx, payouts); err != nil {
				return fmt.Errorf("escrow repository: insert pending payouts: %w", err)
			}
		}
		stored = true
		return nil
	})
	return stored, err
}

func (r *EscrowRepository) SaveTransaction(ctx context.Context, tx domain.Transaction) error {
	return r.db.WithinTx(ctx, func(ctx context.Context) error {
		q := r.q(ctx)
		_, err := q.InsertTransaction(ctx, dbgen.InsertTransactionParams{
			TxHash:     tx.Hash,
			EscrowID:   tx.EscrowID,
			Method:     tx.Method,
			SignerID:   tx.SignerID,
			SenderID:   tx.SenderID,
			ReceiverID: tx.ReceiverID,
			Succeeded:  tx.Succeeded,
			Error:      nullable(tx.Error),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // already stored
		}
		if err != nil {
			return fmt.Errorf("escrow repository: insert transaction: %w", err)
		}
		for i, ev := range tx.Events {
			data := ev.Data
			if len(data) == 0 {
				data = []byte("{}")
			}
			err := q.InsertEvent(ctx, dbgen.InsertEventParams{
				ReceiptID: ev.ReceiptID,
				LogIndex:  int32(ev.LogIndex),
				TxHash:    tx.Hash,
				Position:  int32(i),
				EscrowID:  tx.EscrowID,
				Event:     ev.Event,
				Data:      data,
			})
			if err != nil {
				return fmt.Errorf("escrow repository: insert event: %w", err)
			}
		}
		return nil
	})
}

func (r *EscrowRepository) ListTransactions(ctx context.Context, escrowID string) ([]domain.Transaction, error) {
	q := r.q(ctx)
	rows, err := q.ListTransactions(ctx, escrowID)
	if err != nil {
		return nil, fmt.Errorf("escrow repository: list transactions: %w", err)
	}
	events, err := q.ListEvents(ctx, escrowID)
	if err != nil {
		return nil, fmt.Errorf("escrow repository: list events: %w", err)
	}
	byTx := map[string][]domain.ContractEvent{}
	for _, ev := range events {
		byTx[ev.TxHash] = append(byTx[ev.TxHash], domain.ContractEvent{
			ReceiptID: ev.ReceiptID, LogIndex: int(ev.LogIndex), Event: ev.Event, Data: ev.Data,
		})
	}
	out := make([]domain.Transaction, len(rows))
	for i, row := range rows {
		out[i] = domain.Transaction{
			Hash:       row.TxHash,
			EscrowID:   row.EscrowID,
			Method:     row.Method,
			SignerID:   row.SignerID,
			SenderID:   row.SenderID,
			ReceiverID: row.ReceiverID,
			Succeeded:  row.Succeeded,
			Error:      deref(row.Error),
			Events:     byTx[row.TxHash],
			CreatedAt:  row.CreatedAt,
		}
	}
	return out, nil
}

func (r *EscrowRepository) AddRevisionNote(ctx context.Context, escrowID string, n domain.RevisionNote) error {
	id, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("escrow repository: new id: %w", err)
	}
	err = r.q(ctx).InsertRevisionNote(ctx, dbgen.InsertRevisionNoteParams{
		ID: id, EscrowID: escrowID, Milestone: int32(n.Milestone), Reason: n.Reason, TxHash: n.TxHash,
	})
	if err != nil {
		return fmt.Errorf("escrow repository: insert revision note: %w", err)
	}
	return nil
}

func (r *EscrowRepository) ListRevisionNotes(ctx context.Context, escrowID string) ([]domain.RevisionNote, error) {
	rows, err := r.q(ctx).ListRevisionNotes(ctx, escrowID)
	if err != nil {
		return nil, fmt.Errorf("escrow repository: list revision notes: %w", err)
	}
	out := make([]domain.RevisionNote, len(rows))
	for i, row := range rows {
		out[i] = domain.RevisionNote{Milestone: int(row.Milestone), Reason: row.Reason, TxHash: row.TxHash, CreatedAt: row.CreatedAt}
	}
	return out, nil
}

// ListForAccount returns at most maxAccountEscrows escrows, newest first.
func (r *EscrowRepository) ListForAccount(ctx context.Context, accountID string, f escrow.ListFilter) ([]escrow.AccountEscrow, error) {
	aid, err := uuid.Parse(accountID)
	if err != nil {
		return []escrow.AccountEscrow{}, nil // no account has a malformed ID
	}
	params := dbgen.ListAccountEscrowsParams{AccountID: aid, MaxRows: maxAccountEscrows}
	if f.Role != "" {
		role := string(f.Role)
		params.Role = &role
	}
	if f.Status != "" {
		status := string(f.Status)
		params.Status = &status
	}
	rows, err := r.q(ctx).ListAccountEscrows(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("escrow repository: list account escrows: %w", err)
	}
	out := make([]escrow.AccountEscrow, 0, len(rows))
	for _, row := range rows {
		e, err := r.Get(ctx, row.ID)
		if err != nil {
			return nil, err
		}
		roles := make([]domain.Role, len(row.Roles))
		for i, role := range row.Roles {
			roles[i] = domain.Role(role)
		}
		out = append(out, escrow.AccountEscrow{Escrow: e, Roles: roles, Invited: row.Invited})
	}
	return out, nil
}

func (r *EscrowRepository) ListApproved(ctx context.Context, limit int) ([]string, error) {
	ids, err := r.q(ctx).ListApprovedEscrows(ctx, int32(limit))
	if err != nil {
		return nil, fmt.Errorf("escrow repository: list approved: %w", err)
	}
	return ids, nil
}

func (r *EscrowRepository) ListExpiredSubmissions(ctx context.Context, nowMS uint64, limit int) ([]string, error) {
	ids, err := r.q(ctx).ListExpiredSubmissions(ctx, dbgen.ListExpiredSubmissionsParams{
		DeadlineMs: int64(nowMS), Limit: int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("escrow repository: list expired submissions: %w", err)
	}
	return ids, nil
}

func (r *EscrowRepository) ListDueReleases(ctx context.Context, nowMS uint64, limit int) ([]escrow.MilestoneRef, error) {
	rows, err := r.q(ctx).ListDueReleases(ctx, dbgen.ListDueReleasesParams{NowMs: int64(nowMS), MaxRows: int32(limit)})
	if err != nil {
		return nil, fmt.Errorf("escrow repository: list due releases: %w", err)
	}
	out := make([]escrow.MilestoneRef, len(rows))
	for i, row := range rows {
		out[i] = escrow.MilestoneRef{EscrowID: row.EscrowID, Milestone: uint32(row.Milestone)}
	}
	return out, nil
}

func (r *EscrowRepository) ListPendingPayouts(ctx context.Context, limit int) ([]escrow.PayoutRef, error) {
	rows, err := r.q(ctx).ListDuePayouts(ctx, int32(limit))
	if err != nil {
		return nil, fmt.Errorf("escrow repository: list pending payouts: %w", err)
	}
	out := make([]escrow.PayoutRef, len(rows))
	for i, row := range rows {
		out[i] = escrow.PayoutRef{EscrowID: row.EscrowID, AccountID: row.AccountID}
	}
	return out, nil
}

func (r *EscrowRepository) ListToSync(ctx context.Context, olderThan time.Time, limit int) ([]string, error) {
	ids, err := r.q(ctx).ListEscrowsToSync(ctx, dbgen.ListEscrowsToSyncParams{OlderThan: olderThan, MaxRows: int32(limit)})
	if err != nil {
		return nil, fmt.Errorf("escrow repository: list escrows to sync: %w", err)
	}
	return ids, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func toUint64(v *int64) *uint64 {
	if v == nil {
		return nil
	}
	u := uint64(*v)
	return &u
}

func toInt64(v *uint64) *int64 {
	if v == nil {
		return nil
	}
	i := int64(*v)
	return &i
}
