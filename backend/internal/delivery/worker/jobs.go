package worker

import (
	"context"

	"github.com/harundarat/vecrow/backend/internal/infrastructure/queue"
	"github.com/harundarat/vecrow/backend/internal/usecase/escrow"
	"github.com/harundarat/vecrow/backend/internal/usecase/funding"
)

// JobRunner runs the escrow and funding jobs (jobs.Jobs).
type JobRunner interface {
	Create(ctx context.Context, r escrow.EscrowRef) error
	Sync(ctx context.Context, r escrow.EscrowRef) error
	AutoRelease(ctx context.Context, r escrow.MilestoneRef) error
	RetryPayout(ctx context.Context, r escrow.PayoutRef) error
	CheckFunding(ctx context.Context, c funding.CheckJob) error
}

// JobRoutes binds the job topics to the runner (architecture.md §7.3).
func JobRoutes(r JobRunner) []Route {
	return []Route{
		{Topic: escrow.TopicCreate, Handler: handle(r.Create)},
		{Topic: escrow.TopicSync, Handler: handle(r.Sync)},
		{Topic: escrow.TopicAutoRelease, Handler: handle(r.AutoRelease)},
		{Topic: escrow.TopicRetryPayout, Handler: handle(r.RetryPayout)},
		{Topic: funding.TopicCheck, Handler: handle(r.CheckFunding)},
	}
}

// handle decodes a message's JSON payload and runs fn with it. A malformed
// payload is dead-lettered at once.
func handle[T any](fn func(context.Context, T) error) queue.Handler {
	return func(ctx context.Context, msg queue.Message) error {
		var payload T
		if err := msg.Decode(&payload); err != nil {
			return err
		}
		return fn(ctx, payload)
	}
}
