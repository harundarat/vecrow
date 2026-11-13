package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/harundarat/vecrow/backend/internal/domain"
	"github.com/harundarat/vecrow/backend/internal/infrastructure/database"
	"github.com/harundarat/vecrow/backend/internal/repository/dbgen"
	"github.com/harundarat/vecrow/backend/internal/usecase/auth"
)

// pgUniqueViolation is the SQLSTATE of a unique constraint violation.
const pgUniqueViolation = "23505"

// DB returns the querier for ctx: the transaction started by WithinTx, or the
// pool. *database.Postgres implements it.
type DB interface {
	Querier(ctx context.Context) database.Querier
}

// AccountRepository stores accounts and linked wallets in PostgreSQL.
type AccountRepository struct {
	db DB
}

var _ auth.AccountRepository = (*AccountRepository)(nil)

func NewAccountRepository(db DB) *AccountRepository {
	return &AccountRepository{db: db}
}

func (r *AccountRepository) q(ctx context.Context) *dbgen.Queries {
	return dbgen.New(r.db.Querier(ctx))
}

func (r *AccountRepository) GetAccount(ctx context.Context, id string) (domain.Account, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return domain.Account{}, auth.ErrAccountNotFound
	}
	q := r.q(ctx)
	row, err := q.GetAccount(ctx, uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Account{}, auth.ErrAccountNotFound
	}
	if err != nil {
		return domain.Account{}, fmt.Errorf("account repository: get account: %w", err)
	}
	wallets, err := q.ListWallets(ctx, uid)
	if err != nil {
		return domain.Account{}, fmt.Errorf("account repository: list wallets: %w", err)
	}
	account := domain.Account{
		ID:           row.ID.String(),
		Username:     row.Username,
		Email:        deref(row.Email),
		PendingEmail: deref(row.PendingEmail),
		CreatedAt:    row.CreatedAt,
		Wallets:      make([]domain.LinkedWallet, len(wallets)),
	}
	for i, w := range wallets {
		account.Wallets[i] = domain.LinkedWallet{
			ID:        w.ID.String(),
			Wallet:    domain.Wallet{Scheme: domain.WalletScheme(w.Scheme), Address: w.Address},
			CreatedAt: w.CreatedAt,
		}
	}
	return account, nil
}

func (r *AccountRepository) FindWalletOwner(ctx context.Context, w domain.Wallet) (string, error) {
	owner, err := r.q(ctx).GetWalletOwner(ctx, dbgen.GetWalletOwnerParams{Scheme: string(w.Scheme), Address: w.Address})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", auth.ErrWalletNotFound
	}
	if err != nil {
		return "", fmt.Errorf("account repository: get wallet owner: %w", err)
	}
	return owner.String(), nil
}

func (r *AccountRepository) CreateAccount(ctx context.Context, username string) (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("account repository: new id: %w", err)
	}
	// ON CONFLICT DO NOTHING returns no row for a taken username.
	_, err = r.q(ctx).InsertAccount(ctx, dbgen.InsertAccountParams{ID: id, Username: username})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", auth.ErrUsernameTaken
	}
	if err != nil {
		return "", fmt.Errorf("account repository: insert account: %w", err)
	}
	return id.String(), nil
}

func (r *AccountRepository) LinkWallet(ctx context.Context, accountID string, w domain.Wallet) (bool, error) {
	aid, err := uuid.Parse(accountID)
	if err != nil {
		return false, fmt.Errorf("account repository: invalid account id: %w", err)
	}
	id, err := uuid.NewV7()
	if err != nil {
		return false, fmt.Errorf("account repository: new id: %w", err)
	}
	// ON CONFLICT DO NOTHING returns no row for a wallet that is already linked.
	_, err = r.q(ctx).InsertWallet(ctx, dbgen.InsertWalletParams{
		ID: id, AccountID: aid, Scheme: string(w.Scheme), Address: w.Address,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("account repository: insert wallet: %w", err)
	}
	return true, nil
}

func (r *AccountRepository) UpdateUsername(ctx context.Context, id, username string) error {
	uid, err := uuid.Parse(id)
	if err != nil {
		return auth.ErrAccountNotFound
	}
	n, err := r.q(ctx).UpdateUsername(ctx, dbgen.UpdateUsernameParams{ID: uid, Username: username})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
		return auth.ErrUsernameTaken
	}
	if err != nil {
		return fmt.Errorf("account repository: update username: %w", err)
	}
	if n == 0 {
		return auth.ErrAccountNotFound
	}
	return nil
}

func (r *AccountRepository) LockAccount(ctx context.Context, id string) error {
	uid, err := uuid.Parse(id)
	if err != nil {
		return auth.ErrAccountNotFound
	}
	_, err = r.q(ctx).LockAccount(ctx, uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.ErrAccountNotFound
	}
	if err != nil {
		return fmt.Errorf("account repository: lock account: %w", err)
	}
	return nil
}

func (r *AccountRepository) CountWallets(ctx context.Context, accountID string) (int, error) {
	aid, err := uuid.Parse(accountID)
	if err != nil {
		return 0, fmt.Errorf("account repository: invalid account id: %w", err)
	}
	n, err := r.q(ctx).CountWallets(ctx, aid)
	if err != nil {
		return 0, fmt.Errorf("account repository: count wallets: %w", err)
	}
	return int(n), nil
}

func (r *AccountRepository) UnlinkWallet(ctx context.Context, accountID, walletID string) (bool, error) {
	aid, err1 := uuid.Parse(accountID)
	wid, err2 := uuid.Parse(walletID)
	if err1 != nil || err2 != nil {
		return false, nil // no wallet can match a malformed ID
	}
	n, err := r.q(ctx).DeleteWallet(ctx, dbgen.DeleteWalletParams{ID: wid, AccountID: aid})
	if err != nil {
		return false, fmt.Errorf("account repository: delete wallet: %w", err)
	}
	return n > 0, nil
}

func (r *AccountRepository) FindAccountByEmail(ctx context.Context, email string) (string, error) {
	id, err := r.q(ctx).GetAccountIDByEmail(ctx, &email)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", auth.ErrAccountNotFound
	}
	if err != nil {
		return "", fmt.Errorf("account repository: get account by email: %w", err)
	}
	return id.String(), nil
}

func (r *AccountRepository) SetPendingEmail(ctx context.Context, accountID, email string) error {
	uid, err := uuid.Parse(accountID)
	if err != nil {
		return auth.ErrAccountNotFound
	}
	n, err := r.q(ctx).SetPendingEmail(ctx, dbgen.SetPendingEmailParams{ID: uid, PendingEmail: nullable(email)})
	if err != nil {
		return fmt.Errorf("account repository: set pending email: %w", err)
	}
	if n == 0 {
		return auth.ErrAccountNotFound
	}
	return nil
}

func (r *AccountRepository) ConfirmEmail(ctx context.Context, accountID, email string) (bool, error) {
	uid, err := uuid.Parse(accountID)
	if err != nil {
		return false, nil
	}
	n, err := r.q(ctx).ConfirmEmail(ctx, dbgen.ConfirmEmailParams{ID: uid, PendingEmail: &email})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
		return false, auth.ErrEmailTaken
	}
	if err != nil {
		return false, fmt.Errorf("account repository: confirm email: %w", err)
	}
	return n > 0, nil
}

func (r *AccountRepository) ClearEmail(ctx context.Context, accountID string) error {
	uid, err := uuid.Parse(accountID)
	if err != nil {
		return auth.ErrAccountNotFound
	}
	n, err := r.q(ctx).ClearEmail(ctx, uid)
	if err != nil {
		return fmt.Errorf("account repository: clear email: %w", err)
	}
	if n == 0 {
		return auth.ErrAccountNotFound
	}
	return nil
}

// IsWalletOwner reports whether w is linked to the account.
func (r *AccountRepository) IsWalletOwner(ctx context.Context, accountID string, w domain.Wallet) (bool, error) {
	owner, err := r.FindWalletOwner(ctx, w)
	if errors.Is(err, auth.ErrWalletNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return owner == accountID, nil
}
