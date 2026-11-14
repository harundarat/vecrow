package logger_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/harundarat/vecrow/backend/internal/infrastructure/logger"
)

func newJSONLogger(t *testing.T, level string) (*slog.Logger, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	log, err := logger.New(logger.Config{
		Level: level, Format: "json", Service: "test", Env: "development", Version: "v0", Output: &buf,
	})
	if err != nil {
		t.Fatal(err)
	}
	return log, &buf
}

func decodeLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("invalid JSON log line %q: %v", line, err)
		}
		out = append(out, m)
	}
	return out
}

func TestNew_BaseAttributesAndContext(t *testing.T) {
	log, buf := newJSONLogger(t, "info")

	ctx := logger.WithAttrs(context.Background(), slog.String("request_id", "req-1"))
	ctx = logger.WithAttrs(ctx, slog.String("topic", "jobs"))
	log.InfoContext(ctx, "hello", slog.Int("n", 1))
	log.DebugContext(ctx, "filtered out by level")

	lines := decodeLines(t, buf)
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1: %s", len(lines), buf)
	}
	got := lines[0]
	for k, want := range map[string]any{
		"msg": "hello", "service": "test", "env": "development", "version": "v0",
		"request_id": "req-1", "topic": "jobs", "n": float64(1),
	} {
		if got[k] != want {
			t.Errorf("%s = %v, want %v", k, got[k], want)
		}
	}
}

func TestNew_RedactsSensitiveKeys(t *testing.T) {
	log, buf := newJSONLogger(t, "info")
	log.Info("login",
		slog.String("token", "s3cr3t"),
		slog.String("Authorization", "Bearer abc"),
		slog.String("otp", "123456"),
		slog.Group("req", slog.String("signature", "0xsig")),
		slog.String("address", "alice.near"),
	)

	out := buf.String()
	for _, secret := range []string{"s3cr3t", "Bearer abc", "123456", "0xsig"} {
		if strings.Contains(out, secret) {
			t.Errorf("log output leaks %q: %s", secret, out)
		}
	}
	if !strings.Contains(out, "alice.near") {
		t.Errorf("non-sensitive attribute was removed: %s", out)
	}
}

func TestNew_InvalidConfig(t *testing.T) {
	if _, err := logger.New(logger.Config{Level: "verbose", Format: "json"}); err == nil {
		t.Error("expected error for invalid level")
	}
	if _, err := logger.New(logger.Config{Level: "info", Format: "xml"}); err == nil {
		t.Error("expected error for invalid format")
	}
}

func TestContextAttrsStayTopLevelUnderGroup(t *testing.T) {
	log, buf := newJSONLogger(t, "info")
	ctx := logger.WithAttrs(context.Background(), slog.String("request_id", "req-1"))
	log.WithGroup("payout").With(slog.String("id", "p1")).InfoContext(ctx, "hello", slog.Int("n", 1))

	lines := decodeLines(t, buf)
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1: %s", len(lines), buf)
	}
	got := lines[0]
	if got["request_id"] != "req-1" {
		t.Errorf("request_id = %v, want top-level req-1: %s", got["request_id"], buf)
	}
	group, _ := got["payout"].(map[string]any)
	if group["id"] != "p1" || group["n"] != float64(1) || group["request_id"] != nil {
		t.Errorf("payout group = %v, want id and n only", group)
	}
}
