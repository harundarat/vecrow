package config_test

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/harundarat/vecrow/backend/internal/config"
)

// configKeys are unset before each test so values exported by the shell or
// the Makefile (which includes .env) cannot change the outcome.
var configKeys = []string{
	"APP_ENV", "LOG_LEVEL", "LOG_FORMAT", "HTTP_ADDR", "HTTP_READ_TIMEOUT", "HTTP_WRITE_TIMEOUT",
	"HTTP_IDLE_TIMEOUT", "HTTP_SHUTDOWN_TIMEOUT", "CORS_ALLOWED_ORIGINS", "TRUSTED_PROXIES",
	"DATABASE_URL", "DATABASE_MAX_CONNS", "DATABASE_MIN_CONNS", "DATABASE_MAX_CONN_LIFETIME",
	"DATABASE_MAX_CONN_IDLE_TIME", "CACHE_URL", "QUEUE_URL", "QUEUE_PREFETCH", "QUEUE_MAX_RETRIES",
	"QUEUE_RETRY_DELAY", "QUEUE_HANDLER_TIMEOUT", "SCHEDULER_INTERVAL", "AUTH_NONCE_TTL",
	"AUTH_DOMAIN", "AUTH_SESSION_TTL", "AUTH_GUEST_TOKEN_TTL", "NEAR_RPC_URL", "NEAR_EXPLORER_TX_URL",
	"ESCROW_CONTRACT_ID", "ESCROW_FEE_BPS", "ESCROW_TERMS_TTL", "ESCROW_EXPIRY_GRACE", "USDC_CONTRACT_ID",
	"APP_URL", "RELAYER_ACCOUNT_ID", "RELAYER_PRIVATE_KEY", "RELAYER_MAX_GAS_TGAS", "STORAGE_ACCOUNT_ID",
	"STORAGE_PRIVATE_KEY", "ONECLICK_URL", "ONECLICK_API_KEY", "ONECLICK_SLIPPAGE_BPS", "ONECLICK_QUOTE_TTL",
	"ONECLICK_WATCH_GRACE", "ONECLICK_REFRESH_AFTER", "ONECLICK_TOKENS_TTL", "JOB_SYNC_INTERVAL",
	"JOB_FUNDING_INTERVAL", "JOB_BATCH_SIZE", "JOB_LEASE_TTL", "SMTP_HOST", "SMTP_PORT", "SMTP_USERNAME",
	"SMTP_PASSWORD", "SMTP_TLS", "SMTP_TIMEOUT", "SMTP_FROM",
}

func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range configKeys {
		t.Setenv(k, "") // restores the original value after the test
		os.Unsetenv(k)
	}
}

func setRequired(t *testing.T) {
	t.Helper()
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgres://u:p@localhost:5432/db")
	t.Setenv("CACHE_URL", "redis://localhost:6379/0")
}

func TestLoad_Defaults(t *testing.T) {
	setRequired(t)
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.App.Env != config.EnvDevelopment || cfg.Log.Format != "text" || cfg.HTTP.Addr != ":8080" {
		t.Errorf("unexpected defaults: %+v", cfg)
	}
	if cfg.Auth.NonceTTL != 5*time.Minute {
		t.Errorf("nonce ttl = %v, want 5m", cfg.Auth.NonceTTL)
	}
	if cfg.Auth.SessionTTL != 24*time.Hour {
		t.Errorf("session ttl = %v, want 24h", cfg.Auth.SessionTTL)
	}
}

func TestLoad_ProductionDefaultsToJSON(t *testing.T) {
	setRequired(t)
	t.Setenv("APP_ENV", "production")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Log.Format != "json" {
		t.Errorf("log format = %q, want json", cfg.Log.Format)
	}
}

func TestLoad_Errors(t *testing.T) {
	t.Run("missing required", func(t *testing.T) {
		clearEnv(t)
		t.Setenv("DATABASE_URL", "")
		t.Setenv("CACHE_URL", "")
		if _, err := config.Load(); err == nil {
			t.Fatal("expected error for missing DATABASE_URL and CACHE_URL")
		}
	})
	t.Run("invalid values", func(t *testing.T) {
		setRequired(t)
		t.Setenv("APP_ENV", "staging")
		t.Setenv("LOG_LEVEL", "loud")
		_, err := config.Load()
		if err == nil || !strings.Contains(err.Error(), "APP_ENV") || !strings.Contains(err.Error(), "LOG_LEVEL") {
			t.Fatalf("error = %v, want APP_ENV and LOG_LEVEL errors", err)
		}
	})
}

func TestLoad_CORSOrigins(t *testing.T) {
	t.Run("unset uses the development default", func(t *testing.T) {
		setRequired(t)
		cfg, err := config.Load()
		if err != nil || len(cfg.HTTP.CORSAllowedOrigins) != 1 || cfg.HTTP.CORSAllowedOrigins[0] != "http://localhost:5173" {
			t.Fatalf("origins = %v, err = %v", cfg.HTTP.CORSAllowedOrigins, err)
		}
	})
	t.Run("empty disables CORS", func(t *testing.T) {
		setRequired(t)
		t.Setenv("CORS_ALLOWED_ORIGINS", "")
		cfg, err := config.Load()
		if err != nil || len(cfg.HTTP.CORSAllowedOrigins) != 0 {
			t.Fatalf("origins = %v, err = %v", cfg.HTTP.CORSAllowedOrigins, err)
		}
	})
	for _, bad := range []string{"localhost:5173", "http://a.com, http://b.com", "http://a.com,"} {
		t.Run("rejects "+bad, func(t *testing.T) {
			setRequired(t)
			t.Setenv("CORS_ALLOWED_ORIGINS", bad)
			if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "CORS_ALLOWED_ORIGINS") {
				t.Fatalf("error = %v, want CORS_ALLOWED_ORIGINS error", err)
			}
		})
	}
}

func TestLoad_RejectsNonPositiveDurations(t *testing.T) {
	for _, key := range []string{
		"HTTP_READ_TIMEOUT", "HTTP_WRITE_TIMEOUT", "HTTP_IDLE_TIMEOUT", "HTTP_SHUTDOWN_TIMEOUT",
		"DATABASE_MAX_CONN_LIFETIME", "DATABASE_MAX_CONN_IDLE_TIME", "AUTH_SESSION_TTL",
	} {
		for _, v := range []string{"0s", "-1s"} {
			t.Run(key+"="+v, func(t *testing.T) {
				setRequired(t)
				t.Setenv(key, v)
				if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), key) {
					t.Fatalf("error = %v, want %s error", err, key)
				}
			})
		}
	}
}

func TestLoad_AuthAndNEAR(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		setRequired(t)
		t.Setenv("AUTH_DOMAIN", "app.vecrow.xyz")
		t.Setenv("NEAR_RPC_URL", "https://rpc.testnet.near.org")
		cfg, err := config.Load()
		if err != nil || cfg.Auth.Domain != "app.vecrow.xyz" || cfg.NEAR.RPCURL != "https://rpc.testnet.near.org" {
			t.Fatalf("cfg = %+v %+v, err = %v", cfg.Auth, cfg.NEAR, err)
		}
	})
	for key, bad := range map[string]string{
		"AUTH_DOMAIN":  "vecrow.xyz\nWallet: 0xattacker",
		"NEAR_RPC_URL": "rpc.testnet.near.org",
	} {
		t.Run("rejects "+key, func(t *testing.T) {
			setRequired(t)
			t.Setenv(key, bad)
			if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), key) {
				t.Fatalf("error = %v, want %s error", err, key)
			}
		})
	}
}

func setEscrow(t *testing.T) {
	t.Helper()
	setRequired(t)
	for k, v := range map[string]string{
		"NEAR_RPC_URL":        "http://localhost:3030",
		"ESCROW_CONTRACT_ID":  "escrow.test.near",
		"ESCROW_FEE_BPS":      "0",
		"USDC_CONTRACT_ID":    "usdc.test.near",
		"APP_URL":             "http://localhost:5173",
		"RELAYER_ACCOUNT_ID":  "relayer.test.near",
		"RELAYER_PRIVATE_KEY": "ed25519:secret",
	} {
		t.Setenv(k, v)
	}
}

func TestLoad_EscrowDefaults(t *testing.T) {
	setEscrow(t)
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Escrow.FeeBPS == nil || *cfg.Escrow.FeeBPS != 0 {
		t.Errorf("fee bps = %v, want explicit 0", cfg.Escrow.FeeBPS)
	}
	if cfg.Escrow.TermsTTL != 24*time.Hour || cfg.Auth.GuestTokenTTL != 24*time.Hour ||
		cfg.Relayer.MaxGasTGas != 100 || cfg.OneClick.URL != "https://1click.chaindefuser.com" ||
		cfg.OneClick.WatchGrace != 24*time.Hour || cfg.Jobs.BatchSize != 100 {
		t.Errorf("unexpected defaults: %+v %+v %+v %+v", cfg.Escrow, cfg.Relayer.MaxGasTGas, cfg.OneClick, cfg.Jobs)
	}
	if err := cfg.RequireEscrow(false); err != nil {
		t.Errorf("RequireEscrow(false) = %v", err)
	}
	err = cfg.RequireEscrow(true)
	if err == nil || !strings.Contains(err.Error(), "STORAGE_ACCOUNT_ID") || !strings.Contains(err.Error(), "STORAGE_PRIVATE_KEY") {
		t.Errorf("RequireEscrow(true) = %v, want the storage account required", err)
	}
	if strings.Contains(fmt.Sprintf("%v", err), "ed25519:secret") {
		t.Error("error contains the private key")
	}
}

func TestLoad_RequireEscrowListsMissing(t *testing.T) {
	setRequired(t)
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	err = cfg.RequireEscrow(false)
	for _, key := range []string{"NEAR_RPC_URL", "ESCROW_CONTRACT_ID", "ESCROW_FEE_BPS", "USDC_CONTRACT_ID", "APP_URL", "RELAYER_ACCOUNT_ID", "RELAYER_PRIVATE_KEY"} {
		if err == nil || !strings.Contains(err.Error(), key) {
			t.Errorf("error = %v, want %s", err, key)
		}
	}
}

func TestLoad_RejectsInvalidEscrowSettings(t *testing.T) {
	for key, bad := range map[string]string{
		"ESCROW_FEE_BPS":        "10001",
		"RELAYER_MAX_GAS_TGAS":  "301",
		"ONECLICK_SLIPPAGE_BPS": "-1",
		"JOB_BATCH_SIZE":        "0",
		"ESCROW_TERMS_TTL":      "0s",
		"AUTH_GUEST_TOKEN_TTL":  "-1s",
		"JOB_LEASE_TTL":         "0s",
		"APP_URL":               "localhost:5173",
		"NEAR_EXPLORER_TX_URL":  "explorer/txns/",
		"ONECLICK_URL":          "1click.chaindefuser.com",
	} {
		t.Run(key, func(t *testing.T) {
			setEscrow(t)
			t.Setenv(key, bad)
			if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), key) {
				t.Fatalf("error = %v, want %s error", err, key)
			}
		})
	}
}

func TestLoad_Mail(t *testing.T) {
	setRequired(t)
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Mail.SMTPHost != "" || cfg.Mail.SMTPPort != 587 || cfg.Mail.SMTPTLS != "starttls" || cfg.Mail.SMTPTimeout != 10*time.Second {
		t.Errorf("mail defaults = %+v", cfg.Mail)
	}
	if err := cfg.RequireMail(); err != nil {
		t.Errorf("RequireMail in development: %v", err)
	}

	t.Setenv("APP_ENV", "production")
	cfg, err = config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := cfg.RequireMail(); err == nil {
		t.Error("RequireMail accepted production without SMTP_HOST")
	}

	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_FROM", "Vecrow <no-reply@example.com>")
	t.Setenv("SMTP_USERNAME", "vecrow")
	t.Setenv("SMTP_PASSWORD", "not-a-real-password")
	t.Setenv("SMTP_PORT", "465")
	t.Setenv("SMTP_TLS", "tls")
	cfg, err = config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := cfg.RequireMail(); err != nil {
		t.Errorf("RequireMail: %v", err)
	}
	if cfg.Mail.SMTPPort != 465 || cfg.Mail.SMTPTLS != "tls" || cfg.Mail.SMTPPassword != "not-a-real-password" {
		t.Errorf("mail = %+v", cfg.Mail)
	}

	for key, val := range map[string]string{
		"SMTP_FROM":    "",
		"SMTP_PORT":    "0",
		"SMTP_TLS":     "ssl",
		"SMTP_TIMEOUT": "0s",
	} {
		t.Run(key, func(t *testing.T) {
			t.Setenv(key, val)
			if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), key) {
				t.Errorf("Load with %s=%q error = %v, want one naming %s", key, val, err, key)
			}
		})
	}
}
