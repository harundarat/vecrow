package config_test

import (
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
	"AUTH_DOMAIN", "AUTH_SESSION_TTL", "NEAR_RPC_URL",
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
