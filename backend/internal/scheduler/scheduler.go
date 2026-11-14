// Package scheduler periodically runs tasks that find due work in the
// database and enqueue one message per item (architecture.md §7.3).
package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/harundarat/vecrow/backend/internal/infrastructure/logger"
)

// Name is the component name used in logs.
const Name = "scheduler"

// Task finds due work and enqueues it. Run must be idempotent: a task can run
// again before the messages it published are processed.
type Task interface {
	Name() string
	Run(ctx context.Context) error
}

// Scheduler runs every task once at start, then on every tick. Tasks run
// sequentially, so a slow run delays the next tick instead of overlapping it.
type Scheduler struct {
	interval time.Duration
	log      *slog.Logger
	tasks    []Task
}

func New(interval time.Duration, log *slog.Logger, tasks ...Task) *Scheduler {
	return &Scheduler{interval: interval, log: log.With(slog.String("component", Name)), tasks: tasks}
}

// Run blocks until ctx is cancelled.
func (s *Scheduler) Run(ctx context.Context) error {
	if len(s.tasks) == 0 {
		s.log.WarnContext(ctx, "no scheduled tasks registered, scheduler idle")
		<-ctx.Done()
		return nil
	}

	s.log.InfoContext(ctx, "scheduler started", slog.Duration("interval", s.interval), slog.Int("tasks", len(s.tasks)))
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		for _, t := range s.tasks {
			if ctx.Err() != nil {
				break
			}
			s.runTask(ctx, t)
		}
		select {
		case <-ctx.Done():
			s.log.InfoContext(ctx, "scheduler stopped")
			return nil
		case <-ticker.C:
		}
	}
}

func (s *Scheduler) runTask(ctx context.Context, t Task) {
	ctx = logger.WithAttrs(ctx, slog.String("task", t.Name()))
	start := time.Now()
	err := func() (err error) {
		defer func() {
			if p := recover(); p != nil {
				s.log.ErrorContext(ctx, "scheduled task panicked",
					slog.String("panic", fmt.Sprint(p)), slog.String("stack", string(debug.Stack())))
				err = fmt.Errorf("panic: %v", p)
			}
		}()
		return t.Run(ctx)
	}()
	elapsed := slog.Float64("duration_ms", float64(time.Since(start).Microseconds())/1000)
	if err != nil {
		s.log.ErrorContext(ctx, "scheduled task failed", elapsed, logger.Err(err))
		return
	}
	s.log.DebugContext(ctx, "scheduled task completed", elapsed)
}
