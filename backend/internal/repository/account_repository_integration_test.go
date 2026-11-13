//go:build integration

package repository_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/harundarat/vecrow/backend/internal/domain"
	"github.com/harundarat/vecrow/backend/internal/infrastructure/cache"
	"github.com/harundarat/vecrow/backend/internal/infrastructure/database"
	"github.com/harundarat/vecrow/backend/internal/repository"
	"github.com/harundarat/vecrow/backend/internal/usecase/auth"
)

// openDB connects to DATABASE_URL, which must have the migrations applied
// (make migrate-up).
func openDB(t *testing.T) *database.Postgres {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set")
	}
	db, err := database.OpenPostgres(context.Background(), database.Config{
		URL: url, MaxConns: 8, MaxConnLifetime: time.Hour, MaxConnIdleTime: time.Minute, StartupTimeout: 5 * time.Second,
	}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("OpenPostgres: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	var table *string
	if err := db.Querier(context.Background()).QueryRow(context.Background(), `SELECT to_regclass('accounts')::text`).Scan(&table); err != nil || table == nil {
		t.Fatalf("accounts table missing (%v): run make migrate-up", err)
	}
	return db
}

func randomSuffix(t *testing.T) string {
	t.Helper()
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}

// newAccount creates an account deleted (with its wallets) after the test.
func newAccount(t *testing.T, db *database.Postgres, repo *repository.AccountRepository, username string) string {
	t.Helper()
	ctx := context.Background()
	id, err := repo.CreateAccount(ctx, username)
	if err != nil {
		t.Fatalf("CreateAccount(%q): %v", username, err)
	}
	t.Cleanup(func() { _, _ = db.Querier(ctx).Exec(ctx, `DELETE FROM accounts WHERE id = $1`, id) })
	return id
}

func evmWallet(t *testing.T) domain.Wallet {
	return domain.Wallet{Scheme: domain.SchemeEIP191, Address: "0x" + randomSuffix(t) + "00000000000000000000000000000000"[:28]}
}

func TestAccountRepository(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	repo := repository.NewAccountRepository(db)
	sfx := randomSuffix(t)

	id := newAccount(t, db, repo, "it_a"+sfx)
	if _, err := repo.CreateAccount(ctx, "it_a"+sfx); !errors.Is(err, auth.ErrUsernameTaken) {
		t.Fatalf("duplicate username: error = %v, want ErrUsernameTaken", err)
	}

	w1 := evmWallet(t)
	w2 := domain.Wallet{Scheme: domain.SchemeNEP413, Address: "it" + sfx + ".testnet"}
	for _, w := range []domain.Wallet{w1, w2} {
		if linked, err := repo.LinkWallet(ctx, id, w); err != nil || !linked {
			t.Fatalf("LinkWallet(%v) = %v, %v", w, linked, err)
		}
	}

	t.Run("wallet is unique across accounts", func(t *testing.T) {
		other := newAccount(t, db, repo, "it_b"+sfx)
		linked, err := repo.LinkWallet(ctx, other, w1)
		if err != nil || linked {
			t.Fatalf("LinkWallet of an owned wallet = %v, %v; want false, nil", linked, err)
		}
		owner, err := repo.FindWalletOwner(ctx, w1)
		if err != nil || owner != id {
			t.Fatalf("FindWalletOwner = %q, %v; want %q", owner, err, id)
		}
		// Same address under another scheme is a different wallet.
		if _, err := repo.FindWalletOwner(ctx, domain.Wallet{Scheme: domain.SchemeEd25519, Address: w1.Address}); !errors.Is(err, auth.ErrWalletNotFound) {
			t.Fatalf("other scheme: error = %v, want ErrWalletNotFound", err)
		}
	})

	t.Run("get account with wallets", func(t *testing.T) {
		a, err := repo.GetAccount(ctx, id)
		if err != nil || a.Username != "it_a"+sfx || len(a.Wallets) != 2 || a.Wallets[0].Wallet != w1 || a.Wallets[1].Wallet != w2 {
			t.Fatalf("GetAccount = %+v, %v", a, err)
		}
		if _, err := repo.GetAccount(ctx, "00000000-0000-7000-8000-000000000000"); !errors.Is(err, auth.ErrAccountNotFound) {
			t.Fatalf("unknown id: error = %v", err)
		}
		if _, err := repo.GetAccount(ctx, "not-a-uuid"); !errors.Is(err, auth.ErrAccountNotFound) {
			t.Fatalf("malformed id: error = %v", err)
		}
	})

	t.Run("update username", func(t *testing.T) {
		newAccount(t, db, repo, "it_c"+sfx)
		if err := repo.UpdateUsername(ctx, id, "it_c"+sfx); !errors.Is(err, auth.ErrUsernameTaken) {
			t.Fatalf("taken: error = %v, want ErrUsernameTaken", err)
		}
		if err := repo.UpdateUsername(ctx, id, "it_d"+sfx); err != nil {
			t.Fatalf("UpdateUsername: %v", err)
		}
		if err := repo.UpdateUsername(ctx, "00000000-0000-7000-8000-000000000000", "it_e"+sfx); !errors.Is(err, auth.ErrAccountNotFound) {
			t.Fatalf("unknown account: error = %v", err)
		}
	})

	t.Run("conflicts do not abort the transaction", func(t *testing.T) {
		err := db.WithinTx(ctx, func(ctx context.Context) error {
			if _, err := repo.CreateAccount(ctx, "it_d"+sfx); !errors.Is(err, auth.ErrUsernameTaken) {
				t.Errorf("CreateAccount: %v", err)
			}
			if linked, err := repo.LinkWallet(ctx, id, w1); err != nil || linked {
				t.Errorf("LinkWallet: %v, %v", linked, err)
			}
			_, err := repo.CountWallets(ctx, id) // fails if the tx is aborted
			return err
		})
		if err != nil {
			t.Fatalf("transaction: %v", err)
		}
	})

	t.Run("unlink", func(t *testing.T) {
		a, _ := repo.GetAccount(ctx, id)
		if deleted, err := repo.UnlinkWallet(ctx, id, "not-a-uuid"); err != nil || deleted {
			t.Fatalf("malformed id: %v, %v", deleted, err)
		}
		if deleted, err := repo.UnlinkWallet(ctx, id, a.Wallets[1].ID); err != nil || !deleted {
			t.Fatalf("UnlinkWallet: %v, %v", deleted, err)
		}
		if n, err := repo.CountWallets(ctx, id); err != nil || n != 1 {
			t.Fatalf("CountWallets = %d, %v; want 1", n, err)
		}
	})
}

// Two concurrent unlinks of an account's two wallets must leave one wallet:
// the account lock serialises them so the second sees the first's delete.
func TestAccountService_ConcurrentUnlinkKeepsOneWallet(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	repo := repository.NewAccountRepository(db)
	svc := auth.NewAccountService(nil, nil, repo, db, nil, nil)

	id := newAccount(t, db, repo, "it_u"+randomSuffix(t))
	for range 2 {
		if _, err := repo.LinkWallet(ctx, id, evmWallet(t)); err != nil {
			t.Fatal(err)
		}
	}
	a, _ := repo.GetAccount(ctx, id)

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range 2 {
		wg.Go(func() { errs[i] = svc.UnlinkWallet(ctx, id, a.Wallets[i].ID) })
	}
	wg.Wait()

	if n, _ := repo.CountWallets(ctx, id); n != 1 {
		t.Fatalf("wallets left = %d, want 1 (errors: %v)", n, errs)
	}
	if (errs[0] == nil) == (errs[1] == nil) || !errors.Is(errors.Join(errs...), domain.ErrConflict) {
		t.Fatalf("errors = %v, want exactly one ErrConflict", errs)
	}
}

func TestSessionStore_Redis(t *testing.T) {
	url := os.Getenv("CACHE_URL")
	if url == "" {
		t.Skip("CACHE_URL not set")
	}
	kv, err := cache.OpenRedis(context.Background(), cache.Config{URL: url, StartupTimeout: 5 * time.Second}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = kv.Close() })

	ctx := context.Background()
	store := repository.NewSessionStore(kv)
	key := "it-" + randomSuffix(t)
	sess := domain.Session{AccountID: "acct", ExpiresAt: time.Now().Add(time.Second).UTC()}
	if err := store.Save(ctx, key, sess, time.Second); err != nil {
		t.Fatal(err)
	}
	if got, err := store.Get(ctx, key); err != nil || got.AccountID != "acct" {
		t.Fatalf("Get = %+v, %v", got, err)
	}
	time.Sleep(1500 * time.Millisecond)
	if _, err := store.Get(ctx, key); !errors.Is(err, auth.ErrSessionNotFound) {
		t.Fatalf("after TTL: error = %v, want ErrSessionNotFound", err)
	}
}

func TestAccountRepository_Email(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	repo := repository.NewAccountRepository(db)
	alice := newAccount(t, db, repo, "it_a_"+randomSuffix(t))
	bob := newAccount(t, db, repo, "it_b_"+randomSuffix(t))
	email := "it-" + randomSuffix(t) + "@example.com"

	if _, err := repo.FindAccountByEmail(ctx, email); !errors.Is(err, auth.ErrAccountNotFound) {
		t.Fatalf("FindAccountByEmail(unknown) error = %v", err)
	}
	if err := repo.SetPendingEmail(ctx, alice, email); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetAccount(ctx, alice)
	if err != nil || got.Email != "" || got.PendingEmail != email {
		t.Fatalf("after SetPendingEmail: %+v, %v", got, err)
	}
	// A pending email is not unique and matches nothing.
	if err := repo.SetPendingEmail(ctx, bob, email); err != nil {
		t.Fatalf("pending email of two accounts: %v", err)
	}
	if _, err := repo.FindAccountByEmail(ctx, email); !errors.Is(err, auth.ErrAccountNotFound) {
		t.Fatalf("pending email matched: %v", err)
	}
	// Only the pending email can be confirmed.
	if ok, err := repo.ConfirmEmail(ctx, alice, "other-"+email); err != nil || ok {
		t.Fatalf("ConfirmEmail(another email) = %v, %v", ok, err)
	}
	if ok, err := repo.ConfirmEmail(ctx, alice, email); err != nil || !ok {
		t.Fatalf("ConfirmEmail = %v, %v", ok, err)
	}
	got, _ = repo.GetAccount(ctx, alice)
	if got.Email != email || got.PendingEmail != "" {
		t.Fatalf("after ConfirmEmail: %+v", got)
	}
	if id, err := repo.FindAccountByEmail(ctx, email); err != nil || id != alice {
		t.Fatalf("FindAccountByEmail = %q, %v", id, err)
	}
	// A verified email is unique.
	if ok, err := repo.ConfirmEmail(ctx, bob, email); !errors.Is(err, auth.ErrEmailTaken) || ok {
		t.Fatalf("ConfirmEmail(taken) = %v, %v", ok, err)
	}
	// Removing it frees it; bob's pending email can then be confirmed.
	if err := repo.ClearEmail(ctx, alice); err != nil {
		t.Fatal(err)
	}
	if ok, err := repo.ConfirmEmail(ctx, bob, email); err != nil || !ok {
		t.Fatalf("ConfirmEmail after ClearEmail = %v, %v", ok, err)
	}
	if id, _ := repo.FindAccountByEmail(ctx, email); id != bob {
		t.Fatalf("owner = %q, want bob", id)
	}
	// "" clears a pending email.
	if err := repo.SetPendingEmail(ctx, bob, "new-"+email); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetPendingEmail(ctx, bob, ""); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.GetAccount(ctx, bob)
	if got.Email != email || got.PendingEmail != "" {
		t.Fatalf("after clearing the pending email: %+v", got)
	}
	// The table keeps emails in lower case.
	if _, err := db.Querier(ctx).Exec(ctx, `UPDATE accounts SET pending_email = 'Upper@Example.com' WHERE id = $1`, bob); err == nil {
		t.Fatal("an upper-case email was stored")
	}
	if err := repo.SetPendingEmail(ctx, "not-a-uuid", email); !errors.Is(err, auth.ErrAccountNotFound) {
		t.Fatalf("SetPendingEmail(bad id) error = %v", err)
	}
}

func TestOTPStore_Redis(t *testing.T) {
	url := os.Getenv("CACHE_URL")
	if url == "" {
		t.Skip("CACHE_URL not set")
	}
	kv, err := cache.OpenRedis(context.Background(), cache.Config{URL: url, StartupTimeout: 5 * time.Second}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = kv.Close() })

	ctx := context.Background()
	store := repository.NewOTPStore(kv)
	subject := "it-" + randomSuffix(t) + "@example.com"
	rec := auth.OTPRecord{AccountID: "acct", Email: subject, Salt: []byte("s"), MAC: []byte("m"), ExpiresAt: time.Now().Add(time.Second)}
	if err := store.Save(ctx, "login", subject, rec, time.Second); err != nil {
		t.Fatal(err)
	}
	if n, err := store.CountAttempt(ctx, "login", subject, time.Second); err != nil || n != 1 {
		t.Fatalf("CountAttempt = %d, %v", n, err)
	}

	// Two requests with the right code: exactly one consumes it.
	var wg sync.WaitGroup
	var mu sync.Mutex
	consumed := 0
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := store.Consume(ctx, "login", subject); err == nil {
				mu.Lock()
				consumed++
				mu.Unlock()
			} else if !errors.Is(err, auth.ErrOTPNotFound) {
				t.Errorf("Consume: %v", err)
			}
		}()
	}
	wg.Wait()
	if consumed != 1 {
		t.Fatalf("consumed %d times, want 1", consumed)
	}

	// Codes and attempts expire.
	if err := store.Save(ctx, "login", subject, rec, time.Second); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CountAttempt(ctx, "login", subject, time.Second); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1500 * time.Millisecond)
	if _, err := store.Get(ctx, "login", subject); !errors.Is(err, auth.ErrOTPNotFound) {
		t.Fatalf("after TTL: error = %v", err)
	}
	if n, _ := store.CountAttempt(ctx, "login", subject, time.Second); n != 1 {
		t.Fatalf("attempts after TTL = %d, want 1", n)
	}

	limiter := repository.NewRateLimiter(kv)
	key := "otp:" + subject
	for i := range 2 {
		if ok, err := limiter.Allow(ctx, key, 2, time.Second); err != nil || !ok {
			t.Fatalf("event %d = %v, %v", i+1, ok, err)
		}
	}
	if ok, _ := limiter.Allow(ctx, key, 2, time.Second); ok {
		t.Fatal("3rd event allowed")
	}
	time.Sleep(1500 * time.Millisecond)
	if ok, _ := limiter.Allow(ctx, key, 2, time.Second); !ok {
		t.Fatal("the window did not reset")
	}
}
