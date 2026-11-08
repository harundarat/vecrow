// Package middleware holds the HTTP middleware chain. Order (see router.go):
// RequestID → AccessLog → Recovery → BodyLimit → CORS.
package middleware

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/harundarat/vecrow/backend/internal/infrastructure/logger"
	"github.com/harundarat/vecrow/backend/internal/requestid"
)

// RequestID reuses a valid incoming X-Request-ID or generates one, echoes it
// in the response, and stores it in the request context so every log record
// for the request carries request_id.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(requestid.Header)
		if !requestid.Valid(id) {
			id = requestid.New()
		}
		ctx := requestid.With(c.Request.Context(), id)
		ctx = logger.WithAttrs(ctx, slog.String("request_id", id))
		c.Request = c.Request.WithContext(ctx)
		c.Header(requestid.Header, id)
		c.Next()
	}
}
