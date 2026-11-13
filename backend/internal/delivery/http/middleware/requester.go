package middleware

import (
	"context"
	"log/slog"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/harundarat/vecrow/backend/internal/delivery/http/response"
	"github.com/harundarat/vecrow/backend/internal/domain"
	"github.com/harundarat/vecrow/backend/internal/infrastructure/logger"
)

// GuestTokenHeader carries a guest token (R-21).
const GuestTokenHeader = "X-Guest-Token"

// GuestAuthenticator resolves a guest token to what it grants.
type GuestAuthenticator interface {
	Authenticate(ctx context.Context, token string) (domain.GuestToken, error)
}

type guestKey struct{}

// OptionalAuth authenticates the bearer session and the guest token of a
// request when they are present, for endpoints open to anyone with the link.
// A credential that is present but invalid or expired is rejected with 401,
// and a recovery session with 403; requests with none pass through.
func OptionalAuth(sessions SessionAuthenticator, guests GuestAuthenticator) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		if c.Request.Header.Get("Authorization") != "" {
			token, ok := BearerToken(c.Request)
			if !ok {
				c.Header("WWW-Authenticate", "Bearer")
				response.Error(c, domain.Unauthorized("the Authorization header must be a bearer session token"))
				return
			}
			sess, err := sessions.Authenticate(ctx, token)
			if err != nil {
				c.Header("WWW-Authenticate", `Bearer error="invalid_token"`)
				response.Error(c, err)
				return
			}
			if sess.Recovery {
				response.Error(c, errWalletSessionRequired())
				return
			}
			ctx = context.WithValue(ctx, sessionKey{}, sess)
			ctx = logger.WithAttrs(ctx, slog.String("account_id", sess.AccountID))
		}
		if token := strings.TrimSpace(c.Request.Header.Get(GuestTokenHeader)); token != "" {
			g, err := guests.Authenticate(ctx, token)
			if err != nil {
				response.Error(c, err)
				return
			}
			ctx = context.WithValue(ctx, guestKey{}, g)
		}
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// RequesterFrom returns the session and guest token authenticated by
// RequireSession or OptionalAuth.
func RequesterFrom(ctx context.Context) domain.Requester {
	var req domain.Requester
	if sess, ok := SessionFrom(ctx); ok {
		req.AccountID = sess.AccountID
	}
	if g, ok := ctx.Value(guestKey{}).(domain.GuestToken); ok {
		req.Guest = &g
	}
	return req
}
