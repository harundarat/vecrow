package middleware

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/harundarat/vecrow/backend/internal/delivery/http/response"
)

// RateLimit allows at most limit requests per client IP in each fixed window
// and answers 429 beyond that. Counters live in memory and are dropped at
// the start of every window, so memory is bounded by the number of distinct
// clients seen in one window. Limits are per process.
func RateLimit(limit int, window time.Duration) gin.HandlerFunc {
	var (
		mu       sync.Mutex
		counts   = map[string]int{}
		windowAt = time.Now()
	)
	return func(c *gin.Context) {
		now := time.Now()
		ip := c.ClientIP()

		mu.Lock()
		if now.Sub(windowAt) >= window {
			counts, windowAt = map[string]int{}, now
		}
		counts[ip]++
		n, retryIn := counts[ip], window-now.Sub(windowAt)
		mu.Unlock()

		if n > limit {
			c.Header("Retry-After", strconv.Itoa(int(retryIn.Seconds())+1))
			response.Abort(c, http.StatusTooManyRequests, response.CodeTooManyRequests, "too many requests, try again later")
			return
		}
		c.Next()
	}
}
