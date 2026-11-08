package handler

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/harundarat/vecrow/backend/internal/infrastructure/logger"
)

// Checker reports whether a dependency is usable.
type Checker interface {
	Name() string
	Check(ctx context.Context) error
}

// HealthHandler serves liveness and readiness probes.
type HealthHandler struct {
	log      *slog.Logger
	timeout  time.Duration
	checkers []Checker
}

// NewHealthHandler returns a handler that checks every checker in parallel,
// each bounded by timeout.
func NewHealthHandler(log *slog.Logger, timeout time.Duration, checkers ...Checker) *HealthHandler {
	return &HealthHandler{log: log, timeout: timeout, checkers: checkers}
}

type healthResponse struct {
	Status     string            `json:"status"`
	Components map[string]string `json:"components,omitempty"`
}

func (h *HealthHandler) Live(c *gin.Context) {
	c.JSON(http.StatusOK, healthResponse{Status: "ok"})
}

// Ready reports whether every dependency is reachable (GET /readyz). Failure
// details are logged, not returned, so the probe does not leak internals.
func (h *HealthHandler) Ready(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), h.timeout)
	defer cancel()

	results := make([]error, len(h.checkers))
	var wg sync.WaitGroup
	for i, chk := range h.checkers {
		wg.Go(func() { results[i] = chk.Check(ctx) })
	}
	wg.Wait()

	resp := healthResponse{Status: "ok", Components: make(map[string]string, len(h.checkers))}
	status := http.StatusOK
	for i, chk := range h.checkers {
		if err := results[i]; err != nil {
			resp.Components[chk.Name()] = "down"
			resp.Status = "unavailable"
			status = http.StatusServiceUnavailable
			h.log.WarnContext(ctx, "readiness check failed", slog.String("component", chk.Name()), logger.Err(err))
			continue
		}
		resp.Components[chk.Name()] = "ok"
	}
	c.JSON(status, resp)
}
