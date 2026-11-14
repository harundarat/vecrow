//go:build integration

package repository_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/harundarat/vecrow/backend/internal/domain"
	"github.com/harundarat/vecrow/backend/internal/repository"
	"github.com/harundarat/vecrow/backend/internal/usecase/funding"
)

func TestFundingRepository(t *testing.T) {
	ctx := context.Background()
	f := newEscrowFixture(t)
	acct := newAccount(t, f.db, f.accounts, "it_q"+f.sfx)
	alice := "alice-" + f.sfx + ".test.near"
	e := f.newEscrow(t, acct, alice, []domain.SplitShare{{AccountID: alice, BPS: 10000}})
	repo := repository.NewFundingRepository(f.db)

	if _, err := repo.LatestQuote(ctx, e.ID); !errors.Is(err, funding.ErrNotFound) {
		t.Fatalf("LatestQuote before any = %v", err)
	}
	deadline := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	first := domain.FundingQuote{
		ID: uuid.Must(uuid.NewV7()).String(), EscrowID: e.ID, OriginAsset: "nep141:eth.omft.near", RefundTo: "0xabc",
		DepositAddress: "0xdep1", AmountIn: "10", AmountInFormatted: "0.00001", AmountOut: "1500000",
		Deadline: &deadline, Status: domain.FundingPending, Watching: true, Raw: []byte(`{"quote": {}}`),
	}
	second := first
	second.ID, second.DepositAddress, second.DepositMemo, second.Deadline = uuid.Must(uuid.NewV7()).String(), "0xdep2", "memo", nil
	for _, q := range []domain.FundingQuote{first, second} {
		if err := repo.InsertQuote(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	got, err := repo.GetQuote(ctx, first.ID)
	if err != nil || got.DepositAddress != "0xdep1" || !got.Deadline.Equal(deadline) || !got.Watching ||
		got.CheckedAt != nil || got.OneClickStatus != "" || string(got.Raw) != `{"quote": {}}` {
		t.Fatalf("GetQuote = %+v, %v", got, err)
	}
	latest, err := repo.LatestQuote(ctx, e.ID)
	if err != nil || latest.ID != second.ID || latest.DepositMemo != "memo" || latest.Deadline != nil {
		t.Fatalf("LatestQuote = %+v, %v", latest, err)
	}

	due, _ := repo.ListWatching(ctx, time.Now(), 1000)
	if !slices.Contains(due, first.ID) || !slices.Contains(due, second.ID) {
		t.Fatalf("ListWatching = %v", due)
	}
	if err := repo.UpdateStatus(ctx, first.ID, domain.FundingSuccess, "SUCCESS", false); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateStatus(ctx, second.ID, domain.FundingPending, "PROCESSING", true); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.GetQuote(ctx, first.ID)
	if got.Status != domain.FundingSuccess || got.OneClickStatus != "SUCCESS" || got.Watching || got.CheckedAt == nil {
		t.Fatalf("after UpdateStatus: %+v", got)
	}
	due, _ = repo.ListWatching(ctx, time.Now().Add(-time.Minute), 1000)
	if slices.Contains(due, first.ID) || slices.Contains(due, second.ID) {
		t.Fatalf("ListWatching lists a stopped or just-checked quote: %v", due)
	}
	due, _ = repo.ListWatching(ctx, time.Now().Add(time.Minute), 1000)
	if slices.Contains(due, first.ID) || !slices.Contains(due, second.ID) {
		t.Fatalf("ListWatching = %v", due)
	}
	if err := repo.UpdateStatus(ctx, uuid.NewString(), domain.FundingPending, "", true); !errors.Is(err, funding.ErrNotFound) {
		t.Fatalf("UpdateStatus(unknown) = %v", err)
	}
}
