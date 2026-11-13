//go:build e2e

package e2e_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/harundarat/vecrow/backend/internal/bootstrap"
	"github.com/harundarat/vecrow/backend/internal/config"
	"github.com/harundarat/vecrow/backend/internal/infrastructure/logger"
	"github.com/harundarat/vecrow/backend/internal/neartx"
)

// syncBuffer collects the services' logs.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// createDatabase creates an empty database next to DATABASE_URL's, migrates
// it with goose, and drops it when the test ends.
func createDatabase(t *testing.T) string {
	t.Helper()
	base := os.Getenv("DATABASE_URL")
	if base == "" {
		t.Fatal("DATABASE_URL is not set; run `make compose-up` and `make test-e2e`")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatalf("connect to %s: %v", redactURL(base), err)
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })

	suffix := make([]byte, 6)
	_, _ = rand.Read(suffix)
	name := "vecrow_e2e_" + hex.EncodeToString(suffix)
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("create database: %v", err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)"); err != nil {
			t.Logf("drop database %s: %v", name, err)
		}
	})

	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	dsn := u.String()

	goose := os.Getenv("GOOSE")
	if goose == "" {
		goose = "goose"
	}
	cmd := exec.Command(goose, "-dir", "../migrations", "postgres", dsn, "up")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("goose up: %v\n%s", err, out)
	}
	return dsn
}

func redactURL(s string) string {
	u, err := url.Parse(s)
	if err != nil {
		return "<invalid url>"
	}
	return u.Redacted()
}

// app is the api and the worker, wired by bootstrap from environment
// variables like the binaries, running against the sandbox.
type app struct {
	baseURL string
	logs    *syncBuffer
	secrets []string
}

type appKeys struct {
	relayer, storage ed25519.PrivateKey
}

// startApp sets the environment the api and worker read, loads it with
// config.Load, and runs both until the test ends.
func startApp(t *testing.T, env map[string]string, keys appKeys) *app {
	t.Helper()
	for k, v := range env {
		t.Setenv(k, v)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	logs := &syncBuffer{}
	log, err := logger.New(logger.Config{Level: "debug", Format: "json", Service: "e2e", Output: logs})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	for name, ctor := range map[string]bootstrap.Constructor{"api": bootstrap.NewAPI, "worker": bootstrap.NewWorker} {
		a, err := ctor(ctx, cfg, log.With(slog.String("process", name)))
		if err != nil {
			cancel()
			t.Fatalf("start %s: %v", name, err)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := a.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				t.Errorf("%s stopped: %v", name, err)
			}
			a.Close()
		}()
	}
	t.Cleanup(func() {
		cancel()
		wg.Wait()
		if t.Failed() {
			t.Logf("service logs:\n%s", tail([]byte(logs.String()), 20000))
		}
	})

	a := &app{
		baseURL: "http://" + cfg.HTTP.Addr + "/api/v1",
		logs:    logs,
		secrets: []string{neartx.FormatPrivateKey(keys.relayer), neartx.FormatPrivateKey(keys.storage), oneClickKey},
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		resp, err := http.Get("http://" + cfg.HTTP.Addr + "/readyz")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("api not ready: %v", err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	return a
}

// response is an HTTP response with its body.
type response struct {
	status int
	header http.Header
	body   []byte
}

func (r response) decode(t *testing.T, out any) {
	t.Helper()
	if err := json.Unmarshal(r.body, out); err != nil {
		t.Fatalf("decode %s: %v", r.body, err)
	}
}

// do sends a request; headers are name/value pairs.
func (a *app) do(t *testing.T, method, path string, body any, headers ...string) response {
	t.Helper()
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(mustJSON(t, body))
	}
	req, err := http.NewRequest(method, a.baseURL+path, r)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response{status: resp.StatusCode, header: resp.Header, body: b}
}

// must sends a request and fails unless it returns want.
func (a *app) must(t *testing.T, want int, method, path string, body any, out any, headers ...string) {
	t.Helper()
	res := a.do(t, method, path, body, headers...)
	if res.status != want {
		t.Fatalf("%s %s: status %d, want %d: %s", method, path, res.status, want, res.body)
	}
	if out != nil {
		res.decode(t, out)
	}
}

// eventually polls cond every second until it holds or the timeout passes.
// Once a second keeps the polling within the API's read rate limit.
func eventually(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %s waiting for %s", timeout, what)
		}
		time.Sleep(time.Second)
	}
}

// checkNoSecretsLogged fails if a key, token or signature reached the logs.
func (a *app) checkNoSecretsLogged(t *testing.T, more ...string) {
	t.Helper()
	logs := a.logs.String()
	for _, s := range append(a.secrets, more...) {
		if s != "" && strings.Contains(logs, s) {
			t.Errorf("a secret was logged: %.12s…", s)
		}
	}
}
