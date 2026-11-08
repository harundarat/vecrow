package middleware

import (
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"github.com/harundarat/vecrow/backend/internal/requestid"
)

// CORS allows the frontend origins to call the API. "*" allows every origin
// without credentials; an empty list disables CORS headers entirely. It
// returns an error for a malformed origin (for example one without a scheme)
// instead of panicking like cors.New does.
func CORS(allowedOrigins []string) (gin.HandlerFunc, error) {
	if len(allowedOrigins) == 0 {
		return func(c *gin.Context) { c.Next() }, nil
	}
	cfg := cors.Config{
		AllowMethods: []string{
			http.MethodGet, http.MethodPost, http.MethodPut,
			http.MethodPatch, http.MethodDelete, http.MethodOptions,
		},
		AllowHeaders:  []string{"Origin", "Content-Type", "Authorization", requestid.Header},
		ExposeHeaders: []string{requestid.Header},
		MaxAge:        12 * time.Hour,
	}
	if slices.Contains(allowedOrigins, "*") {
		cfg.AllowAllOrigins = true
	} else {
		cfg.AllowOrigins = allowedOrigins
		cfg.AllowCredentials = true
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("cors: %w", err)
	}
	return cors.New(cfg), nil
}
