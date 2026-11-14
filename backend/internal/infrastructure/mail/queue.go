package mail

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Errors returned by Queue.Send.
var (
	ErrQueueFull   = errors.New("mail: queue is full")
	ErrQueueClosed = errors.New("mail: queue is closed")
)

// QueueConfig configures a Queue.
type QueueConfig struct {
	// Size is how many emails can wait; Send fails beyond it.
	Size int
	// Workers is how many emails are sent at once.
	Workers int
	// DrainTimeout bounds how long Close waits for queued emails.
	DrainTimeout time.Duration
}

// Queue sends emails in the background. Send returns as soon as the email is
// queued, so the time a request takes does not tell whether an email was sent
// (POST /auth/email/otp must not reveal whether an address has an account).
// Failures are logged, without the body.
type Queue struct {
	sender Sender
	log    *slog.Logger
	drain  time.Duration
	jobs   chan job
	wg     sync.WaitGroup

	mu     sync.RWMutex
	closed bool
}

type job struct {
	ctx               context.Context
	to, subject, body string
}

var _ Sender = (*Queue)(nil)

// NewQueue starts the workers; Close stops them.
func NewQueue(sender Sender, log *slog.Logger, cfg QueueConfig) *Queue {
	cfg.Size = max(cfg.Size, 1)
	cfg.Workers = max(cfg.Workers, 1)
	q := &Queue{
		sender: sender,
		log:    log.With(slog.String("component", Name)),
		drain:  cfg.DrainTimeout,
		jobs:   make(chan job, cfg.Size),
	}
	for range cfg.Workers {
		q.wg.Go(q.work)
	}
	return q
}

// Send queues the email. It checks the recipient and subject first, so a
// malformed email is reported to the caller instead of only being logged.
func (q *Queue) Send(ctx context.Context, to, subject, body string) error {
	if _, err := parseAddress(to); err != nil {
		return err
	}
	if err := checkSubject(subject); err != nil {
		return err
	}
	q.mu.RLock()
	defer q.mu.RUnlock()
	if q.closed {
		return ErrQueueClosed
	}
	// The email outlives the request, but keeps its values (request ID).
	j := job{ctx: context.WithoutCancel(ctx), to: to, subject: subject, body: body}
	select {
	case q.jobs <- j:
		return nil
	default:
		return ErrQueueFull
	}
}

func (q *Queue) work() {
	for j := range q.jobs {
		if err := q.sender.Send(j.ctx, j.to, j.subject, j.body); err != nil {
			q.log.ErrorContext(j.ctx, "email not sent",
				slog.String("to", maskAddress(j.to)),
				slog.String("subject", j.subject),
				slog.String("error", err.Error()))
			continue
		}
		q.log.DebugContext(j.ctx, "email sent",
			slog.String("to", maskAddress(j.to)), slog.String("subject", j.subject))
	}
}

// Close stops accepting emails and waits up to the drain timeout for the
// queued ones to be sent.
func (q *Queue) Close() error {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return nil
	}
	q.closed = true
	pending := len(q.jobs)
	close(q.jobs)
	q.mu.Unlock()

	done := make(chan struct{})
	go func() {
		q.wg.Wait()
		close(done)
	}()
	if q.drain <= 0 {
		<-done
		return nil
	}
	select {
	case <-done:
		return nil
	case <-time.After(q.drain):
		return fmt.Errorf("mail: gave up waiting for %d queued emails after %s", pending, q.drain)
	}
}
