package middleware

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"

	"github.com/harundarat/vecrow/backend/internal/delivery/http/response"
)

// Recovery turns a panic into a 500 response and logs it with its stack. The
// panic is attached to the gin context only when the response was already
// written, so the access log (which it runs inside) flags that request as
// failed even though its status says otherwise.
func Recovery(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			// http.ErrAbortHandler deliberately aborts the response; let
			// net/http handle it.
			if err, ok := rec.(error); ok && errors.Is(err, http.ErrAbortHandler) {
				panic(rec)
			}
			log.ErrorContext(c.Request.Context(), "panic recovered",
				slog.String("panic", fmt.Sprint(rec)),
				slog.String("stack", string(debug.Stack())),
			)
			if c.Writer.Written() {
				// The status is already sent; record the failure so the
				// access log does not report the request as a success.
				_ = c.Error(fmt.Errorf("panic after response was written: %v", rec))
				c.Abort()
				return
			}
			response.Abort(c, http.StatusInternalServerError, response.CodeInternal, "internal server error")
		}()
		c.Next()
	}
}
