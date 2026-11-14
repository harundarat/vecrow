// Package retry waits for an infrastructure dependency to become reachable at
// startup (for example while docker compose is still starting containers).
package retry

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/harundarat/vecrow/backend/internal/infrastructure/logger"
)

const (
	initialDelay   = 250 * time.Millisecond
	maxDelay       = 5 * time.Second
	attemptTimeout = 5 * time.Second
)

// Until calls fn with exponential backoff until it succeeds or timeout
// elapses. Each failed attempt is logged at warn level.
func Until(ctx context.Context, log *slog.Logger, component string, timeout time.Duration, fn func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	delay := initialDelay
	for attempt := 1; ; attempt++ {
		attemptCtx, cancelAttempt := context.WithTimeout(ctx, attemptTimeout)
		err := fn(attemptCtx)
		cancelAttempt()
		if err == nil {
			return nil
		}

		log.WarnContext(ctx, "dependency not reachable, retrying",
			slog.String("component", component),
			slog.Int("attempt", attempt),
			slog.Duration("retry_in", delay),
			logger.Err(err),
		)
		select {
		case <-ctx.Done():
			return fmt.Errorf("%s: not reachable after %d attempts: %w", component, attempt, err)
		case <-time.After(delay):
		}
		delay = min(delay*2, maxDelay)
	}
}
