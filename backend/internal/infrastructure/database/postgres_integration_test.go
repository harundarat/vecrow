//go:build integration

package database_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/harundarat/vecrow/backend/internal/infrastructure/database"
)

func openPostgres(t *testing.T) *database.Postgres {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set")
	}
	db, err := database.OpenPostgres(context.Background(), database.Config{
		URL: url, MaxConns: 4, MaxConnLifetime: time.Hour, MaxConnIdleTime: time.Minute, StartupTimeout: 5 * time.Second,
	}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("OpenPostgres: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func count(t *testing.T, ctx context.Context, db *database.Postgres) int {
	t.Helper()
	var n int
	if err := db.Querier(ctx).QueryRow(ctx, `SELECT count(*) FROM it_tx_test`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestPostgres_WithinTx(t *testing.T) {
	ctx := context.Background()
	db := openPostgres(t)

	if _, err := db.Querier(ctx).Exec(ctx, `CREATE TABLE IF NOT EXISTS it_tx_test (id int PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.Querier(ctx).Exec(ctx, `DROP TABLE IF EXISTS it_tx_test`) })
	if _, err := db.Querier(ctx).Exec(ctx, `TRUNCATE it_tx_test`); err != nil {
		t.Fatal(err)
	}

	insert := func(ctx context.Context, id int) error {
		_, err := db.Querier(ctx).Exec(ctx, `INSERT INTO it_tx_test (id) VALUES ($1)`, id)
		return err
	}

	t.Run("commit", func(t *testing.T) {
		err := db.WithinTx(ctx, func(ctx context.Context) error {
			if err := insert(ctx, 1); err != nil {
				return err
			}
			// Nested calls join the outer transaction.
			return db.WithinTx(ctx, func(ctx context.Context) error { return insert(ctx, 2) })
		})
		if err != nil {
			t.Fatal(err)
		}
		if n := count(t, ctx, db); n != 2 {
			t.Fatalf("rows = %d, want 2", n)
		}
	})

	t.Run("rollback on error", func(t *testing.T) {
		boom := errors.New("boom")
		err := db.WithinTx(ctx, func(ctx context.Context) error {
			if err := insert(ctx, 3); err != nil {
				return err
			}
			return boom
		})
		if !errors.Is(err, boom) {
			t.Fatalf("error = %v, want boom", err)
		}
		if n := count(t, ctx, db); n != 2 {
			t.Fatalf("rows = %d, want 2 (insert rolled back)", n)
		}
	})

	t.Run("rollback on panic", func(t *testing.T) {
		func() {
			defer func() { _ = recover() }()
			_ = db.WithinTx(ctx, func(ctx context.Context) error {
				_ = insert(ctx, 4)
				panic("boom")
			})
		}()
		if n := count(t, ctx, db); n != 2 {
			t.Fatalf("rows = %d, want 2 (insert rolled back)", n)
		}
	})
}
