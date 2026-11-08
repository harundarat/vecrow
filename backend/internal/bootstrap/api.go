package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/harundarat/vecrow/backend/internal/config"
	deliveryhttp "github.com/harundarat/vecrow/backend/internal/delivery/http"
	"github.com/harundarat/vecrow/backend/internal/delivery/http/handler"
	"github.com/harundarat/vecrow/backend/internal/infrastructure/database"
	"github.com/harundarat/vecrow/backend/internal/repository"
	"github.com/harundarat/vecrow/backend/internal/usecase"
	"github.com/harundarat/vecrow/backend/internal/usecase/auth"
)

const readinessTimeout = 2 * time.Second

var _ usecase.Transactor = (*database.Postgres)(nil)

// API is the REST API process.
type API struct {
	log             *slog.Logger
	server          *http.Server
	shutdownTimeout time.Duration
	closers         closers
}

// NewAPI opens the database and cache and wires the HTTP stack.
func NewAPI(ctx context.Context, cfg config.Config, log *slog.Logger) (_ App, err error) {
	a := &API{log: log, shutdownTimeout: cfg.HTTP.ShutdownTimeout, closers: closers{log: log}}
	defer func() {
		if err != nil {
			a.closers.closeAll()
		}
	}()

	db, kv, err := openStores(ctx, cfg, log, &a.closers)
	if err != nil {
		return nil, err
	}

	nonceStore := repository.NewNonceStore(kv)

	nonceService := auth.NewNonceService(nonceStore, cfg.Auth.NonceTTL)

	router, err := deliveryhttp.NewRouter(
		deliveryhttp.RouterConfig{
			CORSAllowedOrigins: cfg.HTTP.CORSAllowedOrigins,
			TrustedProxies:     cfg.HTTP.TrustedProxies,
		},
		log,
		deliveryhttp.Handlers{
			Health: handler.NewHealthHandler(log, readinessTimeout, db, kv),
			Auth:   handler.NewAuthHandler(nonceService),
		},
	)
	if err != nil {
		return nil, err
	}
	a.server = deliveryhttp.NewServer(deliveryhttp.ServerConfig{
		Addr:         cfg.HTTP.Addr,
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
		IdleTimeout:  cfg.HTTP.IdleTimeout,
	}, router, log)
	return a, nil
}

// Run serves HTTP until ctx is cancelled, then drains in-flight requests.
func (a *API) Run(ctx context.Context) error {
	ln, err := net.Listen("tcp", a.server.Addr)
	if err != nil {
		return fmt.Errorf("http: listen on %s: %w", a.server.Addr, err)
	}
	a.log.InfoContext(ctx, "http server listening", slog.String("addr", ln.Addr().String()))

	serveErr := make(chan error, 1)
	go func() { serveErr <- a.server.Serve(ln) }()

	select {
	case err := <-serveErr:
		return fmt.Errorf("http: serve: %w", err)
	case <-ctx.Done():
	}

	a.log.Info("http server shutting down", slog.Duration("timeout", a.shutdownTimeout))
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), a.shutdownTimeout)
	defer cancel()
	start := time.Now()
	if err := a.server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("http: shutdown: %w", err)
	}
	if err := <-serveErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("http: serve: %w", err)
	}
	a.log.Info("http server stopped", slog.Float64("duration_ms", float64(time.Since(start).Microseconds())/1000))
	return nil
}

// Close releases the database and cache connections.
func (a *API) Close() { a.closers.closeAll() }
