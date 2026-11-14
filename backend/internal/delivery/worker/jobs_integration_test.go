//go:build integration

package worker_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/harundarat/vecrow/backend/internal/delivery/worker"
	"github.com/harundarat/vecrow/backend/internal/infrastructure/cache"
	"github.com/harundarat/vecrow/backend/internal/infrastructure/queue"
	"github.com/harundarat/vecrow/backend/internal/repository"
	"github.com/harundarat/vecrow/backend/internal/usecase/escrow"
	"github.com/harundarat/vecrow/backend/internal/usecase/jobs"
)

// prefixed isolates the test's queues from a worker that may be running:
// every topic gets a unique prefix.
type prefixed struct {
	prefix string
	pub    queue.Publisher
}

func (p prefixed) Publish(ctx context.Context, topic string, payload any) error {
	return p.pub.Publish(ctx, p.prefix+topic, payload)
}

// recorder is an escrow and funding job fake that records calls. gate
// blocks the jobs until closed; fail makes a call fail its first n times.
type recorder struct {
	mu    sync.Mutex
	calls []string
	fail  map[string]int
	gate  chan struct{}
	due   escrow.Due
	quote []string
}

func (r *recorder) run(call string) error {
	<-r.gate
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, call)
	if r.fail[call] > 0 {
		r.fail[call]--
		return escrow.ErrNotDue
	}
	return nil
}

func (r *recorder) count(call string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, c := range r.calls {
		if c == call {
			n++
		}
	}
	return n
}

func (r *recorder) FindDue(context.Context, time.Duration, int) (escrow.Due, error) {
	return r.due, nil
}
func (r *recorder) CreateOnChain(_ context.Context, id string) error { return r.run("create " + id) }
func (r *recorder) Sync(_ context.Context, id string) error          { return r.run("sync " + id) }
func (r *recorder) AutoRelease(_ context.Context, id string, m uint32) error {
	return r.run(fmt.Sprintf("auto_release %s:%d", id, m))
}
func (r *recorder) RetryPayout(_ context.Context, id, account string) error {
	return r.run("retry_payout " + id + ":" + account)
}

type fundingRecorder struct{ r *recorder }

func (f fundingRecorder) FindDue(context.Context, time.Duration, int) ([]string, error) {
	return f.r.quote, nil
}
func (f fundingRecorder) Check(_ context.Context, id string) error { return f.r.run("check " + id) }

func open(t *testing.T) (*queue.RabbitMQ, *cache.Redis) {
	t.Helper()
	qURL, cURL := os.Getenv("QUEUE_URL"), os.Getenv("CACHE_URL")
	if qURL == "" || cURL == "" {
		t.Skip("QUEUE_URL and CACHE_URL must be set")
	}
	log := slog.New(slog.DiscardHandler)
	q, err := queue.OpenRabbitMQ(context.Background(), queue.RabbitMQConfig{
		URL: qURL, Exchange: "vecrow.it", ConnectionName: "vecrow-jobs-integration-test",
		Prefetch: 4, MaxRetries: 10, RetryDelay: 200 * time.Millisecond,
		HandlerTimeout: 10 * time.Second, StartupTimeout: 5 * time.Second,
	}, log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = q.Close() })
	kv, err := cache.OpenRedis(context.Background(), cache.Config{URL: cURL, StartupTimeout: 5 * time.Second}, log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = kv.Close() })
	return q, kv
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestJobsThroughRabbitMQ runs the scheduler tasks and the consumers over a
// real broker and cache: each due item is published once while queued,
// processed once, retried after a failure, and published again once done.
func TestJobsThroughRabbitMQ(t *testing.T) {
	q, kv := open(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sfx := uuid.NewString()
	e1, e2, e3, e4, e5 := "e1-"+sfx, "e2-"+sfx, "e3-"+sfx, "e4-"+sfx, "e5-"+sfx
	rec := &recorder{
		gate: make(chan struct{}),
		fail: map[string]int{"create " + e5: 2},
		due: escrow.Due{
			Create:      []string{e1, e5},
			Sync:        []string{e4},
			AutoRelease: []escrow.MilestoneRef{{EscrowID: e2, Milestone: 1}},
			RetryPayout: []escrow.PayoutRef{{EscrowID: e3, AccountID: "member.near"}},
		},
		quote: []string{"q-" + sfx},
	}
	leases := repository.NewJobLeases(kv)
	prefix := "it." + sfx + "."
	j := jobs.New(rec, fundingRecorder{rec}, prefixed{prefix, q}, leases, jobs.Config{
		SyncInterval: time.Minute, FundingInterval: time.Second, BatchSize: 10,
		LeaseTTL: time.Minute, LockTTL: time.Minute,
	}, nil)

	routes := worker.JobRoutes(j)
	for i := range routes {
		routes[i].Topic = prefix + routes[i].Topic
	}
	srv := worker.NewServer(q, slog.New(slog.DiscardHandler), routes...)
	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx) }()

	// While the jobs are held up, scheduling again publishes nothing more.
	for range 2 {
		if err := j.EscrowTask().Run(ctx); err != nil {
			t.Fatal(err)
		}
		if err := j.FundingTask().Run(ctx); err != nil {
			t.Fatal(err)
		}
	}
	close(rec.gate)

	calls := []string{
		"create " + e1, "sync " + e4, "auto_release " + e2 + ":1", "retry_payout " + e3 + ":member.near", "check q-" + sfx,
	}
	for _, c := range calls {
		eventually(t, c, func() bool { return rec.count(c) == 1 })
	}
	// The failing create is retried through the retry queue until it works.
	eventually(t, "create retried", func() bool { return rec.count("create "+e5) == 3 })

	// Every job completed, so its lease is free and the next run publishes
	// the items that are still due.
	for _, key := range []string{"jobs:lease:escrow.create:" + e1, "jobs:lease:escrow.create:" + e5, "jobs:lease:funding.check:q-" + sfx} {
		eventually(t, "lease "+key+" freed", func() bool {
			_, err := kv.Get(ctx, key)
			return errors.Is(err, cache.ErrNotFound)
		})
	}
	if err := j.FundingTask().Run(ctx); err != nil {
		t.Fatal(err)
	}
	eventually(t, "second funding check", func() bool { return rec.count("check q-"+sfx) == 2 })
	for _, c := range calls[:4] {
		if n := rec.count(c); n != 1 {
			t.Errorf("%s ran %d times", c, n)
		}
	}

	cancel()
	if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
		t.Errorf("server: %v", err)
	}
	cleanupQueues(t, routes)
}

// TestJobWaitsForTheEscrowLock checks that a job retries while another job
// holds its escrow's lock, and runs once the lock is free.
func TestJobWaitsForTheEscrowLock(t *testing.T) {
	q, kv := open(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sfx := uuid.NewString()
	id := "e-" + sfx
	rec := &recorder{gate: make(chan struct{}), due: escrow.Due{AutoRelease: []escrow.MilestoneRef{{EscrowID: id}}}}
	close(rec.gate)
	leases := repository.NewJobLeases(kv)
	prefix := "it." + sfx + "."
	j := jobs.New(rec, fundingRecorder{rec}, prefixed{prefix, q}, leases, jobs.Config{
		SyncInterval: time.Minute, FundingInterval: time.Second, BatchSize: 10,
		LeaseTTL: time.Minute, LockTTL: time.Minute,
	}, nil)
	routes := worker.JobRoutes(j)
	for i := range routes {
		routes[i].Topic = prefix + routes[i].Topic
	}

	lock := "jobs:lock:escrow:" + id
	token, ok, err := leases.Acquire(ctx, lock, time.Minute)
	if err != nil || !ok {
		t.Fatalf("take lock: %v %v", ok, err)
	}
	t.Cleanup(func() { _ = kv.Delete(context.Background(), lock) })

	srv := worker.NewServer(q, slog.New(slog.DiscardHandler), routes...)
	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx) }()
	if err := j.EscrowTask().Run(ctx); err != nil {
		t.Fatal(err)
	}

	time.Sleep(500 * time.Millisecond) // at least two attempts while locked
	if n := rec.count("auto_release " + id + ":0"); n != 0 {
		t.Fatalf("ran %d times while locked", n)
	}
	if err := leases.Release(ctx, lock, token); err != nil {
		t.Fatal(err)
	}
	eventually(t, "auto_release after unlock", func() bool { return rec.count("auto_release "+id+":0") == 1 })

	cancel()
	<-done
	cleanupQueues(t, routes)
}

// cleanupQueues deletes the test's queues: the topic, retry and dead-letter
// queue of each route.
func cleanupQueues(t *testing.T, routes []worker.Route) {
	t.Helper()
	conn, err := amqp.Dial(os.Getenv("QUEUE_URL"))
	if err != nil {
		t.Logf("cleanup: %v", err)
		return
	}
	defer conn.Close()
	ch, err := conn.Channel()
	if err != nil {
		t.Logf("cleanup: %v", err)
		return
	}
	defer ch.Close()
	for _, rt := range routes {
		for _, name := range []string{rt.Topic, rt.Topic + ".retry", rt.Topic + ".dlq"} {
			if _, err := ch.QueueDelete(name, false, false, false); err != nil {
				t.Logf("cleanup %s: %v", name, err)
				return
			}
		}
	}
}
