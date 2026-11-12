package jobs_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/harundarat/vecrow/backend/internal/domain"
	"github.com/harundarat/vecrow/backend/internal/usecase/escrow"
	"github.com/harundarat/vecrow/backend/internal/usecase/funding"
	"github.com/harundarat/vecrow/backend/internal/usecase/jobs"
)

// memLeases is an in-memory jobs.Leases without expiry.
type memLeases struct {
	mu   sync.Mutex
	held map[string]string
	ttls map[string]time.Duration
	next int
}

func newLeases() *memLeases {
	return &memLeases{held: map[string]string{}, ttls: map[string]time.Duration{}}
}

func (l *memLeases) Acquire(_ context.Context, key string, ttl time.Duration) (string, bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, ok := l.held[key]; ok {
		return "", false, nil
	}
	l.next++
	token := string(rune('a' + l.next))
	l.held[key], l.ttls[key] = token, ttl
	return token, true, nil
}

func (l *memLeases) Release(_ context.Context, key, token string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if token == "" || l.held[key] == token {
		delete(l.held, key)
	}
	return nil
}

func (l *memLeases) isHeld(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, ok := l.held[key]
	return ok
}

type published struct {
	topic   string
	payload any
}

type memPublisher struct {
	msgs []published
	err  error
}

func (p *memPublisher) Publish(_ context.Context, topic string, payload any) error {
	if p.err != nil {
		return p.err
	}
	p.msgs = append(p.msgs, published{topic, payload})
	return nil
}

// fakeEscrows records the jobs it ran.
type fakeEscrows struct {
	due   escrow.Due
	err   error
	calls []string
	// during runs inside a job, to observe the lock.
	during func()
}

func (f *fakeEscrows) FindDue(context.Context, time.Duration, int) (escrow.Due, error) {
	return f.due, nil
}

func (f *fakeEscrows) run(call string) error {
	f.calls = append(f.calls, call)
	if f.during != nil {
		f.during()
	}
	return f.err
}

func (f *fakeEscrows) CreateOnChain(_ context.Context, id string) error { return f.run("create " + id) }
func (f *fakeEscrows) Sync(_ context.Context, id string) error          { return f.run("sync " + id) }
func (f *fakeEscrows) AutoRelease(_ context.Context, id string, m uint32) error {
	return f.run("auto_release " + id + ":" + string(rune('0'+m)))
}
func (f *fakeEscrows) RetryPayout(_ context.Context, id, account string) error {
	return f.run("retry_payout " + id + ":" + account)
}

type fakeFunding struct {
	due     []string
	checked []string
	err     error
}

func (f *fakeFunding) FindDue(context.Context, time.Duration, int) ([]string, error) {
	return f.due, nil
}
func (f *fakeFunding) Check(_ context.Context, id string) error {
	f.checked = append(f.checked, id)
	return f.err
}

var cfg = jobs.Config{
	SyncInterval: time.Minute, FundingInterval: 30 * time.Second, BatchSize: 10,
	LeaseTTL: 2 * time.Minute, LockTTL: 5 * time.Minute,
}

type env struct {
	escrows *fakeEscrows
	funding *fakeFunding
	pub     *memPublisher
	leases  *memLeases
	jobs    *jobs.Jobs
}

func newEnv() *env {
	e := &env{escrows: &fakeEscrows{}, funding: &fakeFunding{}, pub: &memPublisher{}, leases: newLeases()}
	e.jobs = jobs.New(e.escrows, e.funding, e.pub, e.leases, cfg, nil)
	return e
}

func topics(msgs []published) []string {
	out := make([]string, len(msgs))
	for i, m := range msgs {
		out[i] = m.topic
	}
	return out
}

func TestEscrowTaskPublishesEachDueItemOnce(t *testing.T) {
	e := newEnv()
	e.escrows.due = escrow.Due{
		Create:      []string{"e1"},
		Sync:        []string{"e2"},
		AutoRelease: []escrow.MilestoneRef{{EscrowID: "e3", Milestone: 1}},
		RetryPayout: []escrow.PayoutRef{{EscrowID: "e4", AccountID: "member.near"}},
	}
	task := e.jobs.EscrowTask()
	if task.Name() == "" {
		t.Fatal("task has no name")
	}
	if err := task.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{escrow.TopicCreate, escrow.TopicAutoRelease, escrow.TopicRetryPayout, escrow.TopicSync}
	if got := topics(e.pub.msgs); !slices.Equal(got, want) {
		t.Fatalf("topics = %v, want %v", got, want)
	}
	if p := e.pub.msgs[1].payload; p != (escrow.MilestoneRef{EscrowID: "e3", Milestone: 1}) {
		t.Errorf("auto_release payload = %+v", p)
	}
	if p := e.pub.msgs[2].payload; p != (escrow.PayoutRef{EscrowID: "e4", AccountID: "member.near"}) {
		t.Errorf("retry_payout payload = %+v", p)
	}

	// The leases hold until the jobs complete: a second run publishes nothing.
	if err := task.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(e.pub.msgs) != 4 {
		t.Fatalf("second run published %d more", len(e.pub.msgs)-4)
	}

	// A completed job frees its lease; a failed one keeps it.
	if err := e.jobs.Create(context.Background(), escrow.EscrowRef{EscrowID: "e1"}); err != nil {
		t.Fatal(err)
	}
	e.escrows.err = errors.New("rpc down")
	if err := e.jobs.Sync(context.Background(), escrow.EscrowRef{EscrowID: "e2"}); err == nil {
		t.Fatal("failed sync returned nil")
	}
	if err := task.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := topics(e.pub.msgs[4:]); !slices.Equal(got, []string{escrow.TopicCreate}) {
		t.Errorf("third run published %v, want only the completed create", got)
	}
}

func TestPublishFailureFreesTheLease(t *testing.T) {
	e := newEnv()
	e.escrows.due = escrow.Due{Create: []string{"e1"}}
	e.pub.err = errors.New("broker down")
	if err := e.jobs.EscrowTask().Run(context.Background()); err == nil {
		t.Fatal("expected publish error")
	}
	e.pub.err = nil
	if err := e.jobs.EscrowTask().Run(context.Background()); err != nil || len(e.pub.msgs) != 1 {
		t.Fatalf("retry published %d, err %v", len(e.pub.msgs), err)
	}
}

func TestFundingTaskAndCheck(t *testing.T) {
	e := newEnv()
	e.funding.due = []string{"q1", "q2"}
	if err := e.jobs.FundingTask().Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(e.pub.msgs) != 2 || e.pub.msgs[0].topic != funding.TopicCheck || e.pub.msgs[0].payload != (funding.CheckJob{QuoteID: "q1"}) {
		t.Fatalf("published %+v", e.pub.msgs)
	}
	if err := e.jobs.CheckFunding(context.Background(), funding.CheckJob{QuoteID: "q1"}); err != nil {
		t.Fatal(err)
	}
	// q1 was checked, so it is published again at its next interval; q2 is
	// still queued.
	if err := e.jobs.FundingTask().Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(e.pub.msgs) != 3 || e.pub.msgs[2].payload != (funding.CheckJob{QuoteID: "q1"}) {
		t.Fatalf("published %+v", e.pub.msgs)
	}
	if !slices.Equal(e.funding.checked, []string{"q1"}) {
		t.Errorf("checked %v", e.funding.checked)
	}
}

func TestTransactionJobsHoldTheEscrowLock(t *testing.T) {
	e := newEnv()
	ctx := context.Background()
	held := false
	e.escrows.during = func() { held = e.leases.isHeld("jobs:lock:escrow:e1") }

	for name, run := range map[string]func() error{
		"create":       func() error { return e.jobs.Create(ctx, escrow.EscrowRef{EscrowID: "e1"}) },
		"auto_release": func() error { return e.jobs.AutoRelease(ctx, escrow.MilestoneRef{EscrowID: "e1", Milestone: 0}) },
		"retry_payout": func() error { return e.jobs.RetryPayout(ctx, escrow.PayoutRef{EscrowID: "e1", AccountID: "a.near"}) },
	} {
		held = false
		if err := run(); err != nil || !held {
			t.Errorf("%s: err %v, lock held %v", name, err, held)
		}
		if e.leases.isHeld("jobs:lock:escrow:e1") {
			t.Errorf("%s: lock not released", name)
		}
	}
	if e.leases.ttls["jobs:lock:escrow:e1"] != cfg.LockTTL {
		t.Errorf("lock ttl = %v", e.leases.ttls["jobs:lock:escrow:e1"])
	}

	// A busy escrow is retried later, without running the job.
	if _, ok, _ := e.leases.Acquire(ctx, "jobs:lock:escrow:e1", time.Minute); !ok {
		t.Fatal("could not take the lock")
	}
	before := len(e.escrows.calls)
	if err := e.jobs.AutoRelease(ctx, escrow.MilestoneRef{EscrowID: "e1"}); !errors.Is(err, jobs.ErrBusy) {
		t.Fatalf("busy: err %v", err)
	}
	if len(e.escrows.calls) != before {
		t.Error("job ran while the escrow was busy")
	}

	// Sync sends nothing and needs no lock.
	if err := e.jobs.Sync(ctx, escrow.EscrowRef{EscrowID: "e1"}); err != nil {
		t.Errorf("sync while busy: %v", err)
	}
}

func TestJobErrors(t *testing.T) {
	e := newEnv()
	ctx := context.Background()

	e.escrows.err = escrow.ErrNotDue
	if err := e.jobs.Create(ctx, escrow.EscrowRef{EscrowID: "e1"}); !errors.Is(err, escrow.ErrNotDue) {
		t.Errorf("not due: err %v, want a retry", err)
	}
	e.escrows.err = domain.NotFound("escrow not found")
	if err := e.jobs.Create(ctx, escrow.EscrowRef{EscrowID: "gone"}); err != nil {
		t.Errorf("missing escrow: err %v, want the job dropped", err)
	}
	e.funding.err = errors.New("1Click down")
	if err := e.jobs.CheckFunding(ctx, funding.CheckJob{QuoteID: "q1"}); err == nil {
		t.Error("failed check returned nil")
	}
}
