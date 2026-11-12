package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/harundarat/vecrow/backend/internal/domain"
	"github.com/harundarat/vecrow/backend/internal/repository/dbgen"
	"github.com/harundarat/vecrow/backend/internal/usecase/funding"
)

// FundingRepository stores 1Click quotes and their funding status in
// PostgreSQL.
type FundingRepository struct {
	db DB
}

var _ funding.Repository = (*FundingRepository)(nil)

func NewFundingRepository(db DB) *FundingRepository {
	return &FundingRepository{db: db}
}

func (r *FundingRepository) q(ctx context.Context) *dbgen.Queries {
	return dbgen.New(r.db.Querier(ctx))
}

func (r *FundingRepository) InsertQuote(ctx context.Context, q domain.FundingQuote) error {
	id, err := uuid.Parse(q.ID)
	if err != nil {
		return fmt.Errorf("funding repository: invalid quote id: %w", err)
	}
	raw := q.Raw
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	err = r.q(ctx).InsertFundingQuote(ctx, dbgen.InsertFundingQuoteParams{
		ID:                id,
		EscrowID:          q.EscrowID,
		OriginAsset:       q.OriginAsset,
		RefundTo:          q.RefundTo,
		DepositAddress:    q.DepositAddress,
		DepositMemo:       nullable(q.DepositMemo),
		AmountIn:          q.AmountIn,
		AmountInFormatted: q.AmountInFormatted,
		AmountOut:         q.AmountOut,
		Deadline:          q.Deadline,
		Quote:             raw,
		Status:            string(q.Status),
		Watching:          q.Watching,
	})
	if err != nil {
		return fmt.Errorf("funding repository: insert quote: %w", err)
	}
	return nil
}

func (r *FundingRepository) GetQuote(ctx context.Context, id string) (domain.FundingQuote, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return domain.FundingQuote{}, funding.ErrNotFound
	}
	row, err := r.q(ctx).GetFundingQuote(ctx, uid)
	return toFundingQuote(row, err)
}

func (r *FundingRepository) LatestQuote(ctx context.Context, escrowID string) (domain.FundingQuote, error) {
	row, err := r.q(ctx).GetLatestFundingQuote(ctx, escrowID)
	return toFundingQuote(row, err)
}

func toFundingQuote(row dbgen.FundingQuote, err error) (domain.FundingQuote, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.FundingQuote{}, funding.ErrNotFound
	}
	if err != nil {
		return domain.FundingQuote{}, fmt.Errorf("funding repository: get quote: %w", err)
	}
	return domain.FundingQuote{
		ID:                row.ID.String(),
		EscrowID:          row.EscrowID,
		OriginAsset:       row.OriginAsset,
		RefundTo:          row.RefundTo,
		DepositAddress:    row.DepositAddress,
		DepositMemo:       deref(row.DepositMemo),
		AmountIn:          row.AmountIn,
		AmountInFormatted: row.AmountInFormatted,
		AmountOut:         row.AmountOut,
		Deadline:          row.Deadline,
		Status:            domain.FundingStatus(row.Status),
		OneClickStatus:    deref(row.OneclickStatus),
		Watching:          row.Watching,
		CheckedAt:         row.CheckedAt,
		Raw:               row.Quote,
		CreatedAt:         row.CreatedAt,
	}, nil
}

func (r *FundingRepository) UpdateStatus(ctx context.Context, id string, status domain.FundingStatus, oneClickStatus string, watching bool) error {
	uid, err := uuid.Parse(id)
	if err != nil {
		return funding.ErrNotFound
	}
	n, err := r.q(ctx).UpdateFundingStatus(ctx, dbgen.UpdateFundingStatusParams{
		ID: uid, Status: string(status), OneclickStatus: nullable(oneClickStatus), Watching: watching,
	})
	if err != nil {
		return fmt.Errorf("funding repository: update status: %w", err)
	}
	if n == 0 {
		return funding.ErrNotFound
	}
	return nil
}

func (r *FundingRepository) ListWatching(ctx context.Context, checkedBefore time.Time, limit int) ([]string, error) {
	ids, err := r.q(ctx).ListWatchedQuotes(ctx, dbgen.ListWatchedQuotesParams{CheckedBefore: checkedBefore, MaxRows: int32(limit)})
	if err != nil {
		return nil, fmt.Errorf("funding repository: list watched quotes: %w", err)
	}
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id.String()
	}
	return out, nil
}
