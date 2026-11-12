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
	NEAR      NEAR
	Escrow    Escrow
	Relayer   Relayer
	Storage   Storage
	OneClick  OneClick
	Jobs      Jobs
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
	// Domain names the app in login messages and is the NEP-413 recipient.
	// Required by the API only.
	Domain     string        `env:"AUTH_DOMAIN"`
	SessionTTL time.Duration `env:"AUTH_SESSION_TTL" envDefault:"24h"`
	// GuestTokenTTL is the lifetime of the guest token the other party of an
	// escrow receives (R-21).
	GuestTokenTTL time.Duration `env:"AUTH_GUEST_TOKEN_TTL" envDefault:"24h"`
}

type NEAR struct {
	// RPCURL is the NEAR JSON-RPC endpoint. Required by the api and worker.
	RPCURL string `env:"NEAR_RPC_URL"`
	// ExplorerTxURL prefixes a transaction hash to link it in an explorer.
	ExplorerTxURL string `env:"NEAR_EXPLORER_TX_URL"`
}

// Escrow configures the escrow contract. ContractID, FeeBPS, USDCContractID
// and AppURL are required by the api and worker.
type Escrow struct {
	ContractID string `env:"ESCROW_CONTRACT_ID"`
	// FeeBPS must equal the fee_bps the contract was deployed with; the
	// contract rejects terms with another fee. The contract has no view of
	// it, so it is configured here.
	FeeBPS *uint16 `env:"ESCROW_FEE_BPS"`
	// TermsTTL is how long signed terms stay valid (their deadline).
	TermsTTL time.Duration `env:"ESCROW_TERMS_TTL" envDefault:"24h"`
	// ExpiryGrace is waited after a deadline before an approved escrow not
	// yet on-chain returns to Open.
	ExpiryGrace time.Duration `env:"ESCROW_EXPIRY_GRACE" envDefault:"5m"`
	// USDCContractID is the NEP-141 token the escrows hold.
	USDCContractID string `env:"USDC_CONTRACT_ID"`
	// AppURL is the frontend's base URL; escrow links are APP_URL/escrows/{id}.
	AppURL string `env:"APP_URL"`
}

// Relayer is the account that sends transactions to the escrow contract and
// pays their gas (R-09). Required by the api and worker.
type Relayer struct {
	AccountID string `env:"RELAYER_ACCOUNT_ID"`
	// PrivateKey is "ed25519:<base58>". Never logged.
	PrivateKey string `env:"RELAYER_PRIVATE_KEY"`
	// MaxGasTGas caps the gas of a relayed delegate action.
	MaxGasTGas uint64 `env:"RELAYER_MAX_GAS_TGAS" envDefault:"100"`
}

// Storage is the account that pays the USDC storage deposits (R-20).
// Required by the worker; the api does not register storage.
type Storage struct {
	AccountID  string `env:"STORAGE_ACCOUNT_ID"`
	PrivateKey string `env:"STORAGE_PRIVATE_KEY"`
}

// OneClick configures the 1Click Swap API (R-08).
type OneClick struct {
	URL string `env:"ONECLICK_URL" envDefault:"https://1click.chaindefuser.com"`
	// APIKey is sent as a bearer token; without it quotes pay an extra fee.
	APIKey      string        `env:"ONECLICK_API_KEY"`
	SlippageBPS int           `env:"ONECLICK_SLIPPAGE_BPS" envDefault:"100"`
	QuoteTTL    time.Duration `env:"ONECLICK_QUOTE_TTL" envDefault:"1h"`
	// WatchGrace is how long after a quote's deadline its status is watched.
	WatchGrace time.Duration `env:"ONECLICK_WATCH_GRACE" envDefault:"24h"`
	// RefreshAfter is the age after which GET /escrows/:id/funding reads the
	// status from 1Click again.
	RefreshAfter time.Duration `env:"ONECLICK_REFRESH_AFTER" envDefault:"10s"`
	TokensTTL    time.Duration `env:"ONECLICK_TOKENS_TTL" envDefault:"5m"`
}

// Jobs configures the worker's jobs (architecture.md §7.3).
type Jobs struct {
	// SyncInterval is how often an escrow on-chain is read again.
	SyncInterval time.Duration `env:"JOB_SYNC_INTERVAL" envDefault:"5m"`
	// FundingInterval is how often a watched quote's status is checked.
	FundingInterval time.Duration `env:"JOB_FUNDING_INTERVAL" envDefault:"30s"`
	// BatchSize bounds the items of each kind one scheduler run publishes.
	BatchSize int `env:"JOB_BATCH_SIZE" envDefault:"100"`
	// LeaseTTL is how long a published job is not published again.
	LeaseTTL time.Duration `env:"JOB_LEASE_TTL" envDefault:"2m"`
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
	if c.Auth.SessionTTL <= 0 {
		errs = append(errs, errors.New("AUTH_SESSION_TTL must be > 0"))
	}
	// The domain is printed in the message users sign; whitespace or control
	// characters could forge extra lines.
	if strings.ContainsFunc(c.Auth.Domain, func(r rune) bool { return r <= ' ' || r == 0x7f }) {
		errs = append(errs, fmt.Errorf("AUTH_DOMAIN must not contain spaces or control characters, got %q", c.Auth.Domain))
	}
	for _, u := range []struct{ name, val string }{
		{"NEAR_RPC_URL", c.NEAR.RPCURL},
		{"NEAR_EXPLORER_TX_URL", c.NEAR.ExplorerTxURL},
		{"APP_URL", c.Escrow.AppURL},
		{"ONECLICK_URL", c.OneClick.URL},
	} {
		if u.val != "" && !isHTTPURL(u.val) {
			errs = append(errs, fmt.Errorf("%s must start with http:// or https://", u.name))
		}
	}
	for _, d := range []struct {
		name string
		val  time.Duration
	}{
		{"AUTH_GUEST_TOKEN_TTL", c.Auth.GuestTokenTTL},
		{"ESCROW_TERMS_TTL", c.Escrow.TermsTTL},
		{"ONECLICK_QUOTE_TTL", c.OneClick.QuoteTTL},
		{"ONECLICK_REFRESH_AFTER", c.OneClick.RefreshAfter},
		{"ONECLICK_TOKENS_TTL", c.OneClick.TokensTTL},
		{"JOB_SYNC_INTERVAL", c.Jobs.SyncInterval},
		{"JOB_FUNDING_INTERVAL", c.Jobs.FundingInterval},
		{"JOB_LEASE_TTL", c.Jobs.LeaseTTL},
	} {
		if d.val <= 0 {
			errs = append(errs, fmt.Errorf("%s must be > 0, got %s", d.name, d.val))
		}
	}
	if c.Escrow.ExpiryGrace < 0 || c.OneClick.WatchGrace < 0 {
		errs = append(errs, errors.New("ESCROW_EXPIRY_GRACE and ONECLICK_WATCH_GRACE must be >= 0"))
	}
	if c.Escrow.FeeBPS != nil && *c.Escrow.FeeBPS > maxFeeBPS {
		errs = append(errs, fmt.Errorf("ESCROW_FEE_BPS must be at most %d, got %d", maxFeeBPS, *c.Escrow.FeeBPS))
	}
	if c.Relayer.MaxGasTGas < 1 || c.Relayer.MaxGasTGas > maxGasTGas {
		errs = append(errs, fmt.Errorf("RELAYER_MAX_GAS_TGAS must be between 1 and %d", maxGasTGas))
	}
	if c.OneClick.SlippageBPS < 0 || c.OneClick.SlippageBPS > 10_000 {
		errs = append(errs, errors.New("ONECLICK_SLIPPAGE_BPS must be between 0 and 10000"))
	}
	if c.Jobs.BatchSize < 1 {
		errs = append(errs, errors.New("JOB_BATCH_SIZE must be >= 1"))
	}
	return errors.Join(errs...)
}

const (
	// maxFeeBPS is 100%.
	maxFeeBPS = 10_000
	// maxGasTGas is NEAR's gas limit of a transaction.
	maxGasTGas = 300
)

func isHTTPURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

// RequireEscrow reports the escrow settings the api and worker need that are
// missing. withStorage also requires the storage account.
func (c Config) RequireEscrow(withStorage bool) error {
	var missing []string
	for _, v := range []struct{ name, val string }{
		{"NEAR_RPC_URL", c.NEAR.RPCURL},
		{"ESCROW_CONTRACT_ID", c.Escrow.ContractID},
		{"USDC_CONTRACT_ID", c.Escrow.USDCContractID},
		{"APP_URL", c.Escrow.AppURL},
		{"RELAYER_ACCOUNT_ID", c.Relayer.AccountID},
		{"RELAYER_PRIVATE_KEY", c.Relayer.PrivateKey},
	} {
		if v.val == "" {
			missing = append(missing, v.name)
		}
	}
	if c.Escrow.FeeBPS == nil {
		missing = append(missing, "ESCROW_FEE_BPS")
	}
	if withStorage && c.Storage.AccountID == "" {
		missing = append(missing, "STORAGE_ACCOUNT_ID")
	}
	if withStorage && c.Storage.PrivateKey == "" {
		missing = append(missing, "STORAGE_PRIVATE_KEY")
	}
	if len(missing) > 0 {
		return fmt.Errorf("%s required", strings.Join(missing, ", "))
	}
	return nil
}
