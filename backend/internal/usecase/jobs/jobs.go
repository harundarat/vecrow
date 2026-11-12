// Package jobs runs the backend's background work (architecture.md §7.3):
// scheduler tasks find due work and publish one message per item, and
// consumers process each message with the escrow and funding use cases.
//
// Delivery is at least once and the scheduler may publish an item again, so
// every job is idempotent: the use cases check the current state first and
// the contract rejects calls that are not valid for it. Leases and locks in
// the cache only avoid needless duplicates and wasted gas; correctness never
// depends on them.
package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/harundarat/vecrow/backend/internal/domain"
	"github.com/harundarat/vecrow/backend/internal/usecase/escrow"
	"github.com/harundarat/vecrow/backend/internal/usecase/funding"
)

// Publisher enqueues a JSON payload on a topic.
type Publisher interface {
	Publish(ctx context.Context, topic string, payload any) error
}

// Leases are short-lived exclusive keys in a shared store.
type Leases interface {
	// Acquire takes key for ttl unless someone holds it, returning a token
	// to release it with.
	Acquire(ctx context.Context, key string, ttl time.Duration) (token string, ok bool, err error)
	// Release frees key if it is still held with token; an empty token
	// frees it whoever holds it.
	Release(ctx context.Context, key, token string) error
}

// Escrows are the escrow jobs.
type Escrows interface {
	FindDue(ctx context.Context, syncInterval time.Duration, limit int) (escrow.Due, error)
	CreateOnChain(ctx context.Context, id string) error
	Sync(ctx context.Context, id string) error
	AutoRelease(ctx context.Context, id string, milestone uint32) error
	RetryPayout(ctx context.Context, id, account string) error
}

// Funding is the funding watcher job.
type Funding interface {
	FindDue(ctx context.Context, interval time.Duration, limit int) ([]string, error)
	Check(ctx context.Context, quoteID string) error
}

// Config holds the job settings.
type Config struct {
	// SyncInterval is how often an escrow on-chain is read again.
	SyncInterval time.Duration
	// FundingInterval is how often a watched quote is checked.
	FundingInterval time.Duration
	// BatchSize bounds the items of each kind one run publishes.
	BatchSize int
	// LeaseTTL is how long a published item is not published again, unless
	// its job completes first.
	LeaseTTL time.Duration
	// LockTTL bounds how long a job holds its escrow's lock. It must exceed
	// the longest job, including waiting for a transaction's outcome.
	LockTTL time.Duration
}

// ErrBusy means another job holds the escrow's lock; the message is retried.
var ErrBusy = errors.New("jobs: escrow is busy with another job")

// Jobs schedules and runs the escrow and funding jobs.
type Jobs struct {
	escrows Escrows
	funding Funding
	pub     Publisher
	leases  Leases
	cfg     Config
	log     *slog.Logger
}

func New(escrows Escrows, fundingJobs Funding, pub Publisher, leases Leases, cfg Config, log *slog.Logger) *Jobs {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Jobs{escrows: escrows, funding: fundingJobs, pub: pub, leases: leases, cfg: cfg, log: log}
}

// --- Scheduling -----------------------------------------------------------

// leaseKey names the lease of a published item.
func leaseKey(topic, item string) string { return "jobs:lease:" + topic + ":" + item }

// lockKey names the lock of an escrow, held by jobs that send transactions.
func lockKey(escrowID string) string { return "jobs:lock:escrow:" + escrowID }

func milestoneItem(r escrow.MilestoneRef) string {
	return r.EscrowID + ":" + strconv.FormatUint(uint64(r.Milestone), 10)
}

func payoutItem(r escrow.PayoutRef) string { return r.EscrowID + ":" + r.AccountID }

// publish publishes payload unless the item's lease is held. A failed
// publish frees the lease so the next run tries again.
func (j *Jobs) publish(ctx context.Context, topic, item string, payload any) (bool, error) {
	key := leaseKey(topic, item)
	token, ok, err := j.leases.Acquire(ctx, key, j.cfg.LeaseTTL)
	if err != nil {
		return false, fmt.Errorf("jobs: lease %s: %w", key, err)
	}
	if !ok {
		return false, nil
	}
	if err := j.pub.Publish(ctx, topic, payload); err != nil {
		if rerr := j.leases.Release(ctx, key, token); rerr != nil {
			j.log.WarnContext(ctx, "release job lease failed", slog.String("key", key), slog.String("error", rerr.Error()))
		}
		return false, fmt.Errorf("jobs: publish %s: %w", topic, err)
	}
	return true, nil
}

// EscrowTask finds due escrow work (and expires stale submissions on the
// way) and publishes it.
func (j *Jobs) EscrowTask() Task {
	return task{name: "escrow-jobs", run: j.scheduleEscrows}
}

// FundingTask publishes the quotes due for a status check.
func (j *Jobs) FundingTask() Task {
	return task{name: "funding-watcher", run: j.scheduleFunding}
}

// Task is a scheduler task (scheduler.Task).
type Task interface {
	Name() string
	Run(ctx context.Context) error
}

type task struct {
	name string
	run  func(ctx context.Context) error
}

func (t task) Name() string                  { return t.name }
func (t task) Run(ctx context.Context) error { return t.run(ctx) }

func (j *Jobs) scheduleEscrows(ctx context.Context) error {
	due, err := j.escrows.FindDue(ctx, j.cfg.SyncInterval, j.cfg.BatchSize)
	if err != nil {
		return err
	}
	type item struct {
		topic, key string
		payload    any
	}
	var items []item
	for _, id := range due.Create {
		items = append(items, item{escrow.TopicCreate, id, escrow.EscrowRef{EscrowID: id}})
	}
	for _, r := range due.AutoRelease {
		items = append(items, item{escrow.TopicAutoRelease, milestoneItem(r), r})
	}
	for _, r := range due.RetryPayout {
		items = append(items, item{escrow.TopicRetryPayout, payoutItem(r), r})
	}
	for _, id := range due.Sync {
		items = append(items, item{escrow.TopicSync, id, escrow.EscrowRef{EscrowID: id}})
	}
	published := 0
	for _, it := range items {
		ok, err := j.publish(ctx, it.topic, it.key, it.payload)
		if err != nil {
			return err
		}
		if ok {
			published++
		}
	}
	if published > 0 {
		j.log.InfoContext(ctx, "escrow jobs published", slog.Int("count", published))
	}
	return nil
}

func (j *Jobs) scheduleFunding(ctx context.Context) error {
	ids, err := j.funding.FindDue(ctx, j.cfg.FundingInterval, j.cfg.BatchSize)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := j.publish(ctx, funding.TopicCheck, id, funding.CheckJob{QuoteID: id}); err != nil {
			return err
		}
	}
	return nil
}

// --- Consuming ------------------------------------------------------------

// done frees an item's lease once its job completed, so due work found
// later (such as the next funding check) is published without waiting for
// the lease to expire. A failed job keeps its lease; the queue retries it.
func (j *Jobs) done(ctx context.Context, topic, item string, err error) error {
	if errors.Is(err, domain.ErrNotFound) {
		// The escrow or quote is gone; there is nothing left to do.
		j.log.WarnContext(ctx, "job for a missing record dropped", slog.String("topic", topic), slog.String("item", item))
		err = nil
	}
	if err != nil {
		return err
	}
	// Any token: the lease is the scheduler's, and the job is its end.
	if rerr := j.leases.Release(ctx, leaseKey(topic, item), ""); rerr != nil {
		j.log.WarnContext(ctx, "release job lease failed", slog.String("topic", topic), slog.String("error", rerr.Error()))
	}
	return nil
}

// locked runs fn holding the escrow's lock, so two jobs never send
// transactions for the same escrow at once.
func (j *Jobs) locked(ctx context.Context, escrowID string, fn func(context.Context) error) error {
	key := lockKey(escrowID)
	token, ok, err := j.leases.Acquire(ctx, key, j.cfg.LockTTL)
	if err != nil {
		return fmt.Errorf("jobs: lock escrow %s: %w", escrowID, err)
	}
	if !ok {
		return ErrBusy
	}
	defer func() {
		// The job may have outlived ctx; release regardless.
		if err := j.leases.Release(context.WithoutCancel(ctx), key, token); err != nil {
			j.log.WarnContext(ctx, "release escrow lock failed", slog.String("escrow_id", escrowID), slog.String("error", err.Error()))
		}
	}()
	return fn(ctx)
}

// Create creates an approved escrow on-chain.
func (j *Jobs) Create(ctx context.Context, r escrow.EscrowRef) error {
	err := j.locked(ctx, r.EscrowID, func(ctx context.Context) error { return j.escrows.CreateOnChain(ctx, r.EscrowID) })
	return j.done(ctx, escrow.TopicCreate, r.EscrowID, err)
}

// Sync refreshes an escrow from the contract.
func (j *Jobs) Sync(ctx context.Context, r escrow.EscrowRef) error {
	return j.done(ctx, escrow.TopicSync, r.EscrowID, j.escrows.Sync(ctx, r.EscrowID))
}

// AutoRelease releases a milestone whose review window ended.
func (j *Jobs) AutoRelease(ctx context.Context, r escrow.MilestoneRef) error {
	err := j.locked(ctx, r.EscrowID, func(ctx context.Context) error {
		return j.escrows.AutoRelease(ctx, r.EscrowID, r.Milestone)
	})
	return j.done(ctx, escrow.TopicAutoRelease, milestoneItem(r), err)
}

// RetryPayout retries a pending payout.
func (j *Jobs) RetryPayout(ctx context.Context, r escrow.PayoutRef) error {
	err := j.locked(ctx, r.EscrowID, func(ctx context.Context) error {
		return j.escrows.RetryPayout(ctx, r.EscrowID, r.AccountID)
	})
	return j.done(ctx, escrow.TopicRetryPayout, payoutItem(r), err)
}

// CheckFunding reads a quote's status from 1Click.
func (j *Jobs) CheckFunding(ctx context.Context, c funding.CheckJob) error {
	return j.done(ctx, funding.TopicCheck, c.QuoteID, j.funding.Check(ctx, c.QuoteID))
}
