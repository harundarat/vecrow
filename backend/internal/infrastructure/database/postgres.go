package database

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/harundarat/vecrow/backend/internal/infrastructure/internal/retry"
)

// Config configures the PostgreSQL connection pool.
type Config struct {
	URL             string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
	// StartupTimeout bounds how long OpenPostgres waits for the server.
	StartupTimeout time.Duration
}

// Postgres is a PostgreSQL connection pool with transaction support.
type Postgres struct {
	pool *pgxpool.Pool
}

type txKey struct{}

// OpenPostgres waits until the server answers a ping.
func OpenPostgres(ctx context.Context, cfg Config, log *slog.Logger) (*Postgres, error) {
	log = log.With(slog.String("component", Name))

	pcfg, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		// pgx redacts the password in parse errors.
		return nil, fmt.Errorf("database: parse DATABASE_URL: %w", err)
	}
	pcfg.MaxConns = cfg.MaxConns
	pcfg.MinConns = cfg.MinConns
	pcfg.MaxConnLifetime = cfg.MaxConnLifetime
	pcfg.MaxConnIdleTime = cfg.MaxConnIdleTime
	pcfg.ConnConfig.Tracer = newQueryTracer(log)

	pool, err := pgxpool.NewWithConfig(ctx, pcfg)
	if err != nil {
		return nil, fmt.Errorf("database: create pool: %w", err)
	}
	if err := retry.Until(ctx, log, Name, cfg.StartupTimeout, pool.Ping); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database: %w", err)
	}

	log.InfoContext(ctx, "database connected",
		slog.String("host", pcfg.ConnConfig.Host),
		slog.Int("port", int(pcfg.ConnConfig.Port)),
		slog.String("database", pcfg.ConnConfig.Database),
		slog.String("user", pcfg.ConnConfig.User),
		slog.Int("max_conns", int(pcfg.MaxConns)),
	)
	return &Postgres{pool: pool}, nil
}

// Querier returns the transaction started by WithinTx when ctx carries one,
// otherwise the pool. Repositories call it on every operation so they join
// the caller's transaction transparently.
func (p *Postgres) Querier(ctx context.Context) Querier {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return tx
	}
	return p.pool
}

// WithinTx runs fn in a transaction. It commits when fn returns nil and rolls
// back when fn returns an error or panics. Nested calls join the outer
// transaction.
func (p *Postgres) WithinTx(ctx context.Context, fn func(ctx context.Context) error) (err error) {
	if _, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return fn(ctx)
	}

	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("database: begin transaction: %w", err)
	}
	defer func() {
		if r := recover(); r != nil {
			_ = tx.Rollback(context.WithoutCancel(ctx))
			panic(r)
		}
		if err != nil {
			if rbErr := tx.Rollback(context.WithoutCancel(ctx)); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
				err = errors.Join(err, fmt.Errorf("database: rollback: %w", rbErr))
			}
		}
	}()

	if err = fn(context.WithValue(ctx, txKey{}, tx)); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("database: commit: %w", err)
	}
	return nil
}

func (p *Postgres) Name() string { return Name }

func (p *Postgres) Check(ctx context.Context) error { return p.pool.Ping(ctx) }

func (p *Postgres) Close() error {
	p.pool.Close()
	return nil
}
