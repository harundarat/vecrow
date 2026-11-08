package middleware_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/harundarat/vecrow/backend/internal/delivery/http/middleware"
	"github.com/harundarat/vecrow/backend/internal/infrastructure/logger"
)

func TestRecovery_PanicBecomes500AndIsLogged(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var logs bytes.Buffer
	log, err := logger.New(logger.Config{Level: "info", Format: "json", Output: &logs})
	if err != nil {
		t.Fatal(err)
	}

	r := gin.New()
	r.Use(middleware.RequestID(), middleware.AccessLog(log), middleware.Recovery(log))
	r.GET("/boom", func(*gin.Context) { panic("something broke") })

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/boom", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"code":"internal"`) || strings.Contains(rec.Body.String(), "something broke") {
		t.Errorf("body = %s, want generic internal error", rec.Body)
	}

	out := logs.String()
	for _, want := range []string{`"msg":"panic recovered"`, `"panic":"something broke"`, `"stack":`, `"msg":"http request"`, `"status":500`} {
		if !strings.Contains(out, want) {
			t.Errorf("logs missing %s:\n%s", want, out)
		}
	}
}
