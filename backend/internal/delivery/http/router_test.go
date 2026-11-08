package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	deliveryhttp "github.com/harundarat/vecrow/backend/internal/delivery/http"
	"github.com/harundarat/vecrow/backend/internal/delivery/http/handler"
	"github.com/harundarat/vecrow/backend/internal/domain"
	"github.com/harundarat/vecrow/backend/internal/infrastructure/logger"
	"github.com/harundarat/vecrow/backend/internal/requestid"
)

type fakeIssuer struct {
	nonce domain.Nonce
	err   error
}

func (f fakeIssuer) Issue(context.Context, string, string) (domain.Nonce, error) {
	return f.nonce, f.err
}

type fakeChecker struct {
	name string
	err  error
}

func (f fakeChecker) Name() string                { return f.name }
func (f fakeChecker) Check(context.Context) error { return f.err }

type testServer struct {
	engine *gin.Engine
	logs   *bytes.Buffer
}

func newTestServer(t *testing.T, issuer fakeIssuer, checkers ...handler.Checker) *testServer {
	t.Helper()
	var logs bytes.Buffer
	log, err := logger.New(logger.Config{Level: "debug", Format: "json", Service: "test", Output: &logs})
	if err != nil {
		t.Fatal(err)
	}
	engine, err := deliveryhttp.NewRouter(
		deliveryhttp.RouterConfig{CORSAllowedOrigins: []string{"http://localhost:5173"}},
		log,
		deliveryhttp.Handlers{
			Health: handler.NewHealthHandler(log, time.Second, checkers...),
			Auth:   handler.NewAuthHandler(issuer),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return &testServer{engine: engine, logs: &logs}
}

func (s *testServer) do(method, path, body string, headers ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	s.engine.ServeHTTP(rec, req)
	return rec
}

// accessLog returns the single "http request" log record.
func (s *testServer) accessLog(t *testing.T) map[string]any {
	t.Helper()
	var found []map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(s.logs.String()), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("invalid log line %q: %v", line, err)
		}
		if m["msg"] == "http request" {
			found = append(found, m)
		}
	}
	if len(found) != 1 {
		t.Fatalf("found %d access log records, want 1:\n%s", len(found), s.logs)
	}
	return found[0]
}

type errorEnvelope struct {
	Error struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id"`
	} `json:"error"`
}

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) errorEnvelope {
	t.Helper()
	var env errorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("response is not an error envelope: %s", rec.Body)
	}
	return env
}

func TestIssueNonce_Created(t *testing.T) {
	expires := time.Date(2026, 11, 8, 12, 5, 0, 0, time.UTC)
	srv := newTestServer(t, fakeIssuer{nonce: domain.Nonce{Value: "abc123", ExpiresAt: expires}})

	rec := srv.do(http.MethodPost, "/api/v1/auth/nonce", `{"scheme":"nep413","address":"alice.near"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var body struct {
		Nonce     string    `json:"nonce"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Nonce != "abc123" || !body.ExpiresAt.Equal(expires) {
		t.Fatalf("body = %s (%v)", rec.Body, err)
	}

	reqID := rec.Header().Get(requestid.Header)
	if reqID == "" {
		t.Fatal("missing X-Request-ID response header")
	}
	entry := srv.accessLog(t)
	for k, want := range map[string]any{
		"level": "INFO", "request_id": reqID, "method": "POST",
		"route": "/api/v1/auth/nonce", "status": float64(201),
	} {
		if entry[k] != want {
			t.Errorf("access log %s = %v, want %v", k, entry[k], want)
		}
	}
}

func TestRequestID_ReusesValidIncomingID(t *testing.T) {
	srv := newTestServer(t, fakeIssuer{})
	rec := srv.do(http.MethodGet, "/healthz", "", requestid.Header, "client-id-42")
	if got := rec.Header().Get(requestid.Header); got != "client-id-42" {
		t.Errorf("X-Request-ID = %q, want client-id-42", got)
	}

	srv = newTestServer(t, fakeIssuer{})
	rec = srv.do(http.MethodGet, "/healthz", "", requestid.Header, `bad id"injected`)
	if got := rec.Header().Get(requestid.Header); got == "" || strings.Contains(got, "injected") {
		t.Errorf("X-Request-ID = %q, want a freshly generated ID", got)
	}
}

func TestIssueNonce_ClientErrors(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		issuer     fakeIssuer
		wantStatus int
		wantCode   string
		wantMsg    string
	}{
		{
			name: "missing field", body: `{"scheme":"nep413"}`,
			wantStatus: 400, wantCode: "invalid_argument", wantMsg: "address is required",
		},
		{
			name: "malformed json", body: `{"scheme":`,
			wantStatus: 400, wantCode: "invalid_argument", wantMsg: "request body is not valid JSON",
		},
		{
			name: "empty body", body: "",
			wantStatus: 400, wantCode: "invalid_argument", wantMsg: "request body is required",
		},
		{
			name: "domain error", body: `{"scheme":"nep413","address":"x"}`,
			issuer:     fakeIssuer{err: domain.InvalidArgument("address is not a valid NEAR account ID")},
			wantStatus: 400, wantCode: "invalid_argument", wantMsg: "address is not a valid NEAR account ID",
		},
		{
			name: "body is not an object", body: `[]`,
			wantStatus: 400, wantCode: "invalid_argument", wantMsg: "request body must be a JSON object",
		},
		{
			name: "body too large", body: `{"scheme":"` + strings.Repeat("a", 2<<20) + `"}`,
			wantStatus: 413, wantCode: "payload_too_large",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newTestServer(t, tt.issuer)
			rec := srv.do(http.MethodPost, "/api/v1/auth/nonce", tt.body)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d, body %s", rec.Code, tt.wantStatus, rec.Body)
			}
			env := decodeError(t, rec)
			if env.Error.Code != tt.wantCode || (tt.wantMsg != "" && env.Error.Message != tt.wantMsg) {
				t.Errorf("error = %+v, want code %q message %q", env.Error, tt.wantCode, tt.wantMsg)
			}
			if env.Error.RequestID != rec.Header().Get(requestid.Header) {
				t.Errorf("request_id in body %q != header %q", env.Error.RequestID, rec.Header().Get(requestid.Header))
			}
			entry := srv.accessLog(t)
			if entry["level"] != "WARN" || entry["error"] == nil {
				t.Errorf("access log = %v, want WARN with error", entry)
			}
		})
	}
}

func TestIssueNonce_InternalErrorIsHidden(t *testing.T) {
	srv := newTestServer(t, fakeIssuer{err: errors.New("cache: setnx: dial tcp 10.0.0.5:6379: connection refused")})
	rec := srv.do(http.MethodPost, "/api/v1/auth/nonce", `{"scheme":"nep413","address":"alice.near"}`)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
	env := decodeError(t, rec)
	if env.Error.Code != "internal" || strings.Contains(rec.Body.String(), "10.0.0.5") {
		t.Errorf("internal details leaked to client: %s", rec.Body)
	}
	entry := srv.accessLog(t)
	if entry["level"] != "ERROR" || !strings.Contains(entry["error"].(string), "connection refused") {
		t.Errorf("access log = %v, want ERROR with the internal error", entry)
	}
}

func TestHealth(t *testing.T) {
	t.Run("liveness is logged at debug", func(t *testing.T) {
		srv := newTestServer(t, fakeIssuer{})
		if rec := srv.do(http.MethodGet, "/healthz", ""); rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		if lvl := srv.accessLog(t)["level"]; lvl != "DEBUG" {
			t.Errorf("level = %v, want DEBUG", lvl)
		}
	})

	t.Run("ready", func(t *testing.T) {
		srv := newTestServer(t, fakeIssuer{}, fakeChecker{name: "database"}, fakeChecker{name: "cache"})
		rec := srv.do(http.MethodGet, "/readyz", "")
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"cache":"ok"`) {
			t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
		}
	})

	t.Run("not ready hides the cause", func(t *testing.T) {
		srv := newTestServer(t, fakeIssuer{},
			fakeChecker{name: "database"},
			fakeChecker{name: "cache", err: errors.New("dial tcp 10.0.0.5:6379: connection refused")},
		)
		rec := srv.do(http.MethodGet, "/readyz", "")
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d", rec.Code)
		}
		var body struct {
			Status     string            `json:"status"`
			Components map[string]string `json:"components"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Status != "unavailable" || body.Components["database"] != "ok" || body.Components["cache"] != "down" {
			t.Errorf("body = %+v", body)
		}
		if strings.Contains(rec.Body.String(), "10.0.0.5") {
			t.Error("readiness response leaks the error")
		}
		if !strings.Contains(srv.logs.String(), "readiness check failed") {
			t.Error("readiness failure was not logged")
		}
	})
}

func TestRouting_NotFoundAndMethodNotAllowed(t *testing.T) {
	srv := newTestServer(t, fakeIssuer{})
	if rec := srv.do(http.MethodGet, "/api/v1/nope", ""); rec.Code != 404 || decodeError(t, rec).Error.Code != "not_found" {
		t.Errorf("unknown route: status %d body %s", rec.Code, rec.Body)
	}
	if rec := srv.do(http.MethodGet, "/api/v1/auth/nonce", ""); rec.Code != 405 || decodeError(t, rec).Error.Code != "method_not_allowed" {
		t.Errorf("wrong method: status %d body %s", rec.Code, rec.Body)
	}
}

func TestCORS_Preflight(t *testing.T) {
	srv := newTestServer(t, fakeIssuer{})
	rec := srv.do(http.MethodOptions, "/api/v1/auth/nonce", "",
		"Origin", "http://localhost:5173",
		"Access-Control-Request-Method", "POST",
	)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Errorf("Access-Control-Allow-Origin = %q", got)
	}
}

func TestNewRouter_MalformedCORSOriginIsAnError(t *testing.T) {
	log := slog.New(slog.DiscardHandler)
	_, err := deliveryhttp.NewRouter(
		deliveryhttp.RouterConfig{CORSAllowedOrigins: []string{"localhost:5173"}},
		log,
		deliveryhttp.Handlers{
			Health: handler.NewHealthHandler(log, time.Second),
			Auth:   handler.NewAuthHandler(fakeIssuer{}),
		},
	)
	if err == nil {
		t.Fatal("expected an error, not a panic or success")
	}
}

func TestIssueNonce_RateLimited(t *testing.T) {
	srv := newTestServer(t, fakeIssuer{nonce: domain.Nonce{Value: "n"}})
	body := `{"scheme":"nep413","address":"alice.near"}`
	for i := range 20 {
		if rec := srv.do(http.MethodPost, "/api/v1/auth/nonce", body); rec.Code != http.StatusCreated {
			t.Fatalf("request %d: status = %d, want 201", i+1, rec.Code)
		}
	}
	rec := srv.do(http.MethodPost, "/api/v1/auth/nonce", body)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
	if env := decodeError(t, rec); env.Error.Code != "too_many_requests" {
		t.Errorf("code = %q, want too_many_requests", env.Error.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("missing Retry-After header")
	}
}
