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

// Per-client-IP limits for unauthenticated or signature-checking endpoints:
// nonces are stored in the cache, and verifying a NEAR signature calls the
// NEAR RPC.
const (
	nonceRateLimit  = 20
	verifyRateLimit = 20 // shared by /auth/verify and POST /me/wallets
)

type RouterConfig struct {
	CORSAllowedOrigins []string
	// TrustedProxies are the proxy IPs/CIDRs whose X-Forwarded-For is used
	// for the client IP. Empty trusts none.
	TrustedProxies []string
}

type Handlers struct {
	Health *handler.HealthHandler
	Auth   *handler.AuthHandler
	Me     *handler.MeHandler
	// Sessions authenticates bearer tokens for the routes that need a session.
	Sessions middleware.SessionAuthenticator
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

	requireSession := middleware.RequireSession(h.Sessions)
	verifyLimit := middleware.RateLimit(verifyRateLimit, time.Minute)

	v1 := r.Group(APIPrefix)
	{
		auth := v1.Group("/auth")
		auth.POST("/nonce", middleware.RateLimit(nonceRateLimit, time.Minute), h.Auth.IssueNonce)
		auth.POST("/verify", verifyLimit, h.Auth.Verify)
		auth.POST("/logout", requireSession, h.Auth.Logout)

		me := v1.Group("/me", requireSession)
		me.GET("", h.Me.Get)
		me.PATCH("", h.Me.Update)
		me.POST("/wallets", verifyLimit, h.Me.LinkWallet)
		me.DELETE("/wallets/:id", h.Me.UnlinkWallet)
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
