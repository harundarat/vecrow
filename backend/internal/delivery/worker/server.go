// Package worker is the queue delivery layer: it binds topics to the
// handlers that process them.
package worker

import (
	"context"
	"log/slog"

	"golang.org/x/sync/errgroup"

	"github.com/harundarat/vecrow/backend/internal/infrastructure/queue"
)

// Route binds a topic to the handler that processes its messages.
type Route struct {
	Topic   string
	Handler queue.Handler
}

// Server consumes every route's topic concurrently.
type Server struct {
	sub    queue.Subscriber
	log    *slog.Logger
	routes []Route
}

// NewServer returns a Server for routes.
func NewServer(sub queue.Subscriber, log *slog.Logger, routes ...Route) *Server {
	return &Server{sub: sub, log: log, routes: routes}
}

// Run blocks until ctx is cancelled or a subscription fails permanently.
func (s *Server) Run(ctx context.Context) error {
	if len(s.routes) == 0 {
		s.log.WarnContext(ctx, "no queue routes registered, consumer idle")
		<-ctx.Done()
		return nil
	}

	g, ctx := errgroup.WithContext(ctx)
	for _, rt := range s.routes {
		g.Go(func() error { return s.sub.Subscribe(ctx, rt.Topic, rt.Handler) })
	}
	s.log.InfoContext(ctx, "queue consumers running", slog.Int("routes", len(s.routes)))
	return g.Wait()
}
