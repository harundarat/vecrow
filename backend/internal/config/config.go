// Package config loads the service configuration from environment variables.
// Every variable is documented in backend/.env.example.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

const (
	EnvDevelopment = "development"
	EnvProduction  = "production"
)

// Config is the full configuration shared by the api and worker binaries.
type Config struct {
	App       App
	Log       Log
	HTTP      HTTP
	Database  Database
	Cache     Cache
	Queue     Queue
	Scheduler Scheduler
	Auth      Auth
}

type App struct {
	Env string `env:"APP_ENV" envDefault:"development"`
}

type Log struct {
	Level string `env:"LOG_LEVEL" envDefault:"info"`
	// Format is "json" or "text". Empty means json in production, text otherwise.
	Format string `env:"LOG_FORMAT"`
}

// defaultCORSOrigins applies only when CORS_ALLOWED_ORIGINS is unset; an
// explicitly empty value disables CORS. (env's envDefault would also replace
// an empty value, so the default is applied in Load.)
const defaultCORSOrigins = "http://localhost:5173"

type HTTP struct {
	Addr               string        `env:"HTTP_ADDR" envDefault:":8080"`
	ReadTimeout        time.Duration `env:"HTTP_READ_TIMEOUT" envDefault:"10s"`
	WriteTimeout       time.Duration `env:"HTTP_WRITE_TIMEOUT" envDefault:"15s"`
	IdleTimeout        time.Duration `env:"HTTP_IDLE_TIMEOUT" envDefault:"60s"`
	ShutdownTimeout    time.Duration `env:"HTTP_SHUTDOWN_TIMEOUT" envDefault:"15s"`
	CORSAllowedOrigins []string      `env:"CORS_ALLOWED_ORIGINS" envSeparator:","`
	TrustedProxies     []string      `env:"TRUSTED_PROXIES" envSeparator:","`
}

type Database struct {
	URL             string        `env:"DATABASE_URL,required,notEmpty"`
	MaxConns        int32         `env:"DATABASE_MAX_CONNS" envDefault:"10"`
	MinConns        int32         `env:"DATABASE_MIN_CONNS" envDefault:"0"`
	MaxConnLifetime time.Duration `env:"DATABASE_MAX_CONN_LIFETIME" envDefault:"1h"`
	MaxConnIdleTime time.Duration `env:"DATABASE_MAX_CONN_IDLE_TIME" envDefault:"30m"`
}

type Cache struct {
	URL string `env:"CACHE_URL,required,notEmpty"`
}

type Queue struct {
	// URL is required by the worker only; the API does not connect to the queue.
	URL            string        `env:"QUEUE_URL"`
	Prefetch       int           `env:"QUEUE_PREFETCH" envDefault:"10"`
	MaxRetries     int           `env:"QUEUE_MAX_RETRIES" envDefault:"5"`
	RetryDelay     time.Duration `env:"QUEUE_RETRY_DELAY" envDefault:"30s"`
	HandlerTimeout time.Duration `env:"QUEUE_HANDLER_TIMEOUT" envDefault:"60s"`
}

type Scheduler struct {
	Interval time.Duration `env:"SCHEDULER_INTERVAL" envDefault:"30s"`
}

type Auth struct {
	NonceTTL time.Duration `env:"AUTH_NONCE_TTL" envDefault:"5m"`
}

// Load reads an optional .env file from the working directory, then parses
// the environment. Variables already set in the environment win over .env.
func Load() (Config, error) {
	if err := godotenv.Load(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Config{}, fmt.Errorf("config: load .env: %w", err)
	}
	cfg, err := env.ParseAs[Config]()
	if err != nil {
		return Config{}, fmt.Errorf("config: %w", err)
	}
	if _, set := os.LookupEnv("CORS_ALLOWED_ORIGINS"); !set {
		cfg.HTTP.CORSAllowedOrigins = []string{defaultCORSOrigins}
	}
	if cfg.Log.Format == "" {
		cfg.Log.Format = "text"
		if cfg.IsProduction() {
			cfg.Log.Format = "json"
		}
	}
	if err := cfg.validate(); err != nil {
		return Config{}, fmt.Errorf("config: %w", err)
	}
	return cfg, nil
}

// IsProduction reports whether APP_ENV is production.
func (c Config) IsProduction() bool { return c.App.Env == EnvProduction }

func (c Config) validate() error {
	var errs []error
	if c.App.Env != EnvDevelopment && c.App.Env != EnvProduction {
		errs = append(errs, fmt.Errorf("APP_ENV must be %q or %q, got %q", EnvDevelopment, EnvProduction, c.App.Env))
	}
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(c.Log.Level)); err != nil {
		errs = append(errs, fmt.Errorf("LOG_LEVEL must be debug, info, warn or error, got %q", c.Log.Level))
	}
	if c.Log.Format != "json" && c.Log.Format != "text" {
		errs = append(errs, fmt.Errorf("LOG_FORMAT must be json or text, got %q", c.Log.Format))
	}
	if slices.Contains(c.HTTP.CORSAllowedOrigins, "*") && len(c.HTTP.CORSAllowedOrigins) > 1 {
		errs = append(errs, errors.New(`CORS_ALLOWED_ORIGINS cannot mix "*" with explicit origins`))
	}
	for _, d := range []struct {
		name string
		val  time.Duration
	}{
		{"HTTP_READ_TIMEOUT", c.HTTP.ReadTimeout},
		{"HTTP_WRITE_TIMEOUT", c.HTTP.WriteTimeout},
		{"HTTP_IDLE_TIMEOUT", c.HTTP.IdleTimeout},
		{"HTTP_SHUTDOWN_TIMEOUT", c.HTTP.ShutdownTimeout},
		{"DATABASE_MAX_CONN_LIFETIME", c.Database.MaxConnLifetime},
		{"DATABASE_MAX_CONN_IDLE_TIME", c.Database.MaxConnIdleTime},
	} {
		if d.val <= 0 {
			errs = append(errs, fmt.Errorf("%s must be > 0, got %s", d.name, d.val))
		}
	}
	for _, origin := range c.HTTP.CORSAllowedOrigins {
		if origin != "*" && !strings.HasPrefix(origin, "http://") && !strings.HasPrefix(origin, "https://") {
			errs = append(errs, fmt.Errorf("CORS_ALLOWED_ORIGINS entry %q must be \"*\" or start with http:// or https:// (no spaces)", origin))
		}
	}
	if c.Database.MaxConns < 1 || c.Database.MinConns < 0 || c.Database.MinConns > c.Database.MaxConns {
		errs = append(errs, errors.New("DATABASE_MAX_CONNS must be >= 1 and DATABASE_MIN_CONNS between 0 and DATABASE_MAX_CONNS"))
	}
	if c.Queue.Prefetch < 1 || c.Queue.MaxRetries < 0 || c.Queue.RetryDelay <= 0 || c.Queue.HandlerTimeout <= 0 {
		errs = append(errs, errors.New("QUEUE_PREFETCH must be >= 1, QUEUE_MAX_RETRIES >= 0, QUEUE_RETRY_DELAY and QUEUE_HANDLER_TIMEOUT > 0"))
	}
	if c.Scheduler.Interval <= 0 {
		errs = append(errs, errors.New("SCHEDULER_INTERVAL must be > 0"))
	}
	if c.Auth.NonceTTL <= 0 {
		errs = append(errs, errors.New("AUTH_NONCE_TTL must be > 0"))
	}
	return errors.Join(errs...)
}
