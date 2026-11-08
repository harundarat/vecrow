// Package http is the REST delivery layer: router, middleware, handlers.
// Import it with an alias (deliveryhttp) to avoid clashing with net/http.
package http

import (
	"fmt"
	"log/slog"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"

	"github.com/harundarat/vecrow/backend/internal/delivery/http/handler"
	"github.com/harundarat/vecrow/backend/internal/delivery/http/middleware"
	"github.com/harundarat/vecrow/backend/internal/delivery/http/response"
)

// APIPrefix is the base path of every versioned endpoint.
const APIPrefix = "/api/v1"

// maxBodyBytes caps request bodies.
const maxBodyBytes = 1 << 20

// nonceRateLimit is the number of nonces one client IP may request per minute.
const nonceRateLimit = 20

type RouterConfig struct {
	CORSAllowedOrigins []string
	// TrustedProxies are the proxy IPs/CIDRs whose X-Forwarded-For is used
	// for the client IP. Empty trusts none.
	TrustedProxies []string
}

type Handlers struct {
	Health *handler.HealthHandler
	Auth   *handler.AuthHandler
}

func NewRouter(cfg RouterConfig, log *slog.Logger, h Handlers) (*gin.Engine, error) {
	gin.SetMode(gin.ReleaseMode)
	useJSONFieldNamesInValidationErrors()

	cors, err := middleware.CORS(cfg.CORSAllowedOrigins)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}

	r := gin.New()
	if err := r.SetTrustedProxies(cfg.TrustedProxies); err != nil {
		return nil, fmt.Errorf("http: trusted proxies: %w", err)
	}
	r.HandleMethodNotAllowed = true

	r.Use(
		middleware.RequestID(),
		middleware.AccessLog(log, "/healthz", "/readyz"),
		middleware.Recovery(log),
		middleware.BodyLimit(maxBodyBytes),
		cors,
	)
	r.NoRoute(func(c *gin.Context) {
		response.Abort(c, http.StatusNotFound, response.CodeNotFound, "route not found")
	})
	r.NoMethod(func(c *gin.Context) {
		response.Abort(c, http.StatusMethodNotAllowed, response.CodeMethodNotAllowed, "method not allowed")
	})

	r.GET("/healthz", h.Health.Live)
	r.GET("/readyz", h.Health.Ready)

	v1 := r.Group(APIPrefix)
	{
		// Nonces are unauthenticated and each one is stored in the cache, so
		// issuance is rate limited per client IP.
		auth := v1.Group("/auth")
		auth.POST("/nonce", middleware.RateLimit(nonceRateLimit, time.Minute), h.Auth.IssueNonce)
	}

	for _, rt := range r.Routes() {
		log.Debug("http route registered", slog.String("method", rt.Method), slog.String("path", rt.Path))
	}
	return r, nil
}

type ServerConfig struct {
	Addr         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

// NewServer wraps h in an http.Server with timeouts. net/http's own error
// messages (TLS handshakes, broken connections) are routed to slog.
func NewServer(cfg ServerConfig, h http.Handler, log *slog.Logger) *http.Server {
	return &http.Server{
		Addr:              cfg.Addr,
		Handler:           h,
		ReadTimeout:       cfg.ReadTimeout,
		ReadHeaderTimeout: cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}
}

var registerTagNameOnce sync.Once

// useJSONFieldNamesInValidationErrors makes validation errors name fields as
// clients send them ("address") instead of the Go field name ("Address").
func useJSONFieldNamesInValidationErrors() {
	registerTagNameOnce.Do(func() {
		v, ok := binding.Validator.Engine().(*validator.Validate)
		if !ok {
			return
		}
		v.RegisterTagNameFunc(func(f reflect.StructField) string {
			name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			if name == "-" {
				return ""
			}
			if name == "" {
				return f.Name
			}
			return name
		})
	})
}
