// Package bootstrap is the composition root: it is the only place that knows
// the concrete infrastructure (PostgreSQL, Redis, RabbitMQ) and wires
// repositories, use cases and delivery together by constructor injection.
package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"slices"
	"syscall"
	"time"

	"github.com/harundarat/vecrow/backend/internal/buildinfo"
	"github.com/harundarat/vecrow/backend/internal/config"
	"github.com/harundarat/vecrow/backend/internal/infrastructure/cache"
	"github.com/harundarat/vecrow/backend/internal/infrastructure/database"
	"github.com/harundarat/vecrow/backend/internal/infrastructure/logger"
)

// startupTimeout bounds how long each dependency is awaited at startup.
const startupTimeout = 30 * time.Second

// App is a runnable binary (api or worker).
type App interface {
	// Run blocks until ctx is cancelled, then shuts down gracefully.
	Run(ctx context.Context) error
	// Close releases the resources opened by the constructor.
	Close()
}

// Constructor builds an App from configuration.
type Constructor func(ctx context.Context, cfg config.Config, log *slog.Logger) (App, error)

// Main loads configuration, builds the logger and the app, runs it until
// SIGINT/SIGTERM, and returns the process exit code.
func Main(service string, newApp Constructor) int {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("startup failed", slog.String("service", service), logger.Err(err))
		return 1
	}
	log, err := logger.New(logger.Config{
		Level:   cfg.Log.Level,
		Format:  cfg.Log.Format,
		Service: service,
		Env:     cfg.App.Env,
		Version: buildinfo.Version,
	})
	if err != nil {
		slog.Error("startup failed", slog.String("service", service), logger.Err(err))
		return 1
	}
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// done is closed (deferred after stop, so it runs first) when Main
	// returns, so the cancellation by stop() is not mistaken for a signal.
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			log.Info("shutdown signal received")
			stop() // a second signal kills the process immediately
		case <-done:
		}
	}()

	log.Info("starting",
		slog.String("commit", buildinfo.Commit),
		slog.String("go_version", runtime.Version()),
		slog.String("log_level", cfg.Log.Level),
	)

	app, err := newApp(ctx, cfg, log)
	if err != nil {
		log.Error("startup failed", logger.Err(err))
		return 1
	}

	runErr := app.Run(ctx)
	app.Close()
	if runErr != nil {
		log.Error("stopped with error", logger.Err(runErr))
		return 1
	}
	log.Info("stopped")
	return 0
}

// closers releases resources in reverse order of acquisition.
type closers struct {
	log   *slog.Logger
	items []namedCloser
}

type namedCloser struct {
	name string
	fn   func() error
}

func (c *closers) add(name string, fn func() error) {
	c.items = append(c.items, namedCloser{name: name, fn: fn})
}

func (c *closers) closeAll() {
	for _, item := range slices.Backward(c.items) {
		if err := item.fn(); err != nil {
			c.log.Error("close failed", slog.String("component", item.name), logger.Err(err))
			continue
		}
		c.log.Info("closed", slog.String("component", item.name))
	}
	c.items = nil
}

func wrapStartup(component string, err error) error {
	return fmt.Errorf("open %s: %w", component, err)
}

// openStores opens the database and cache shared by the api and worker and
// registers them with c so they are closed in reverse order.
func openStores(ctx context.Context, cfg config.Config, log *slog.Logger, c *closers) (*database.Postgres, *cache.Redis, error) {
	db, err := database.OpenPostgres(ctx, database.Config{
		URL:             cfg.Database.URL,
		MaxConns:        cfg.Database.MaxConns,
		MinConns:        cfg.Database.MinConns,
		MaxConnLifetime: cfg.Database.MaxConnLifetime,
		MaxConnIdleTime: cfg.Database.MaxConnIdleTime,
		StartupTimeout:  startupTimeout,
	}, log)
	if err != nil {
		return nil, nil, wrapStartup(database.Name, err)
	}
	c.add(database.Name, db.Close)

	kv, err := cache.OpenRedis(ctx, cache.Config{URL: cfg.Cache.URL, StartupTimeout: startupTimeout}, log)
	if err != nil {
		return nil, nil, wrapStartup(cache.Name, err)
	}
	c.add(cache.Name, kv.Close)
	return db, kv, nil
}
