package middleware

import (
	"log/slog"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// AccessLog writes one record per request after it completes:
// quietRoutes (health probes, whose handlers log their own failures) at
// debug, 5xx at error, 4xx at warn, the rest at info. A request that recorded
// an error but still got a success status (a panic after the response was
// written) is logged at error. Bodies, query strings and headers other than User-Agent are never
// logged, because they can carry tokens, signatures or OTP codes.
func AccessLog(log *slog.Logger, quietRoutes ...string) gin.HandlerFunc {
	quiet := make(map[string]struct{}, len(quietRoutes))
	for _, r := range quietRoutes {
		quiet[r] = struct{}{}
	}

	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		status := c.Writer.Status()
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}

		level := slog.LevelInfo
		switch _, isQuiet := quiet[route]; {
		case status < 400 && len(c.Errors) > 0:
			level = slog.LevelError
		case isQuiet:
			level = slog.LevelDebug
		case status >= 500:
			level = slog.LevelError
		case status >= 400:
			level = slog.LevelWarn
		}
		ctx := c.Request.Context()
		if !log.Enabled(ctx, level) {
			return
		}

		attrs := []slog.Attr{
			slog.String("method", c.Request.Method),
			slog.String("route", route),
			slog.String("path", c.Request.URL.Path),
			slog.Int("status", status),
			slog.Float64("latency_ms", float64(time.Since(start).Microseconds())/1000),
			slog.Int("bytes", max(c.Writer.Size(), 0)),
			slog.String("client_ip", c.ClientIP()),
			slog.String("user_agent", c.Request.UserAgent()),
		}
		if len(c.Errors) > 0 {
			msgs := make([]string, len(c.Errors))
			for i, e := range c.Errors {
				msgs[i] = e.Error()
			}
			attrs = append(attrs, slog.String("error", strings.Join(msgs, "; ")))
		}
		log.LogAttrs(ctx, level, "http request", attrs...)
	}
}
