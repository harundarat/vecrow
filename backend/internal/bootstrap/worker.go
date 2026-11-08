package bootstrap

import (
	"context"
	"errors"
	"log/slog"

	"golang.org/x/sync/errgroup"

	"github.com/harundarat/vecrow/backend/internal/config"
	"github.com/harundarat/vecrow/backend/internal/delivery/worker"
	"github.com/harundarat/vecrow/backend/internal/infrastructure/queue"
	"github.com/harundarat/vecrow/backend/internal/scheduler"
)

// Worker is the background process: the scheduler finds due work and
// publishes it, and the consumers process the queued messages.
type Worker struct {
	scheduler *scheduler.Scheduler
	consumers *worker.Server
	closers   closers
}

// NewWorker opens the database, cache and queue and wires the scheduler and
// consumers.
func NewWorker(ctx context.Context, cfg config.Config, log *slog.Logger) (_ App, err error) {
	if cfg.Queue.URL == "" {
		return nil, errors.New("QUEUE_URL is required for the worker")
	}
	w := &Worker{closers: closers{log: log}}
	defer func() {
		if err != nil {
			w.closers.closeAll()
		}
	}()

	// Opened now so a misconfigured worker fails at startup. The worker has
	// no consumers using them yet.
	if _, _, err = openStores(ctx, cfg, log, &w.closers); err != nil {
		return nil, err
	}

	q, err := queue.OpenRabbitMQ(ctx, queue.RabbitMQConfig{
		URL:            cfg.Queue.URL,
		ConnectionName: "vecrow-worker",
		Prefetch:       cfg.Queue.Prefetch,
		MaxRetries:     cfg.Queue.MaxRetries,
		RetryDelay:     cfg.Queue.RetryDelay,
		HandlerTimeout: cfg.Queue.HandlerTimeout,
		StartupTimeout: startupTimeout,
	}, log)
	if err != nil {
		return nil, wrapStartup(queue.Name, err)
	}
	w.closers.add(queue.Name, q.Close)

	w.scheduler = scheduler.New(cfg.Scheduler.Interval, log)

	w.consumers = worker.NewServer(q, log)
	return w, nil
}

// Run runs the scheduler and consumers until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error { return w.scheduler.Run(ctx) })
	g.Go(func() error { return w.consumers.Run(ctx) })
	return g.Wait()
}

// Close releases the queue, cache and database connections.
func (w *Worker) Close() { w.closers.closeAll() }
