package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/harundarat/vecrow/backend/internal/delivery/http/response"
	"github.com/harundarat/vecrow/backend/internal/domain"
	"github.com/harundarat/vecrow/backend/internal/infrastructure/logger"
)

// SessionAuthenticator resolves a bearer token to its session.
type SessionAuthenticator interface {
	Authenticate(ctx context.Context, token string) (domain.Session, error)
}

type sessionKey struct{}

// errWalletSessionRequired rejects a recovery session (R-22) on a route that
// needs a session opened with a wallet signature.
func errWalletSessionRequired() error {
	return domain.Forbidden("this action needs a session signed in with a wallet; " +
		"a session from an email code can only view the account and link or unlink wallets")
}

// RequireSession rejects requests without a valid "Authorization: Bearer
// <token>" header with 401, and recovery sessions with 403, and stores the
// session in the request context for SessionFrom.
func RequireSession(auth SessionAuthenticator) gin.HandlerFunc {
	return requireSession(auth, false)
}

// RequireAnySession is RequireSession that also accepts recovery sessions
// opened with an email code. Use it only for the routes recovery allows:
// viewing the account, linking and unlinking wallets, and logging out.
func RequireAnySession(auth SessionAuthenticator) gin.HandlerFunc {
	return requireSession(auth, true)
}

func requireSession(auth SessionAuthenticator, allowRecovery bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := BearerToken(c.Request)
		if !ok {
			c.Header("WWW-Authenticate", "Bearer")
			response.Error(c, domain.Unauthorized("a bearer session token is required"))
			return
		}
		sess, err := auth.Authenticate(c.Request.Context(), token)
		if err != nil {
			c.Header("WWW-Authenticate", `Bearer error="invalid_token"`)
			response.Error(c, err)
			return
		}
		if sess.Recovery && !allowRecovery {
			response.Error(c, errWalletSessionRequired())
			return
		}
		ctx := context.WithValue(c.Request.Context(), sessionKey{}, sess)
		ctx = logger.WithAttrs(ctx, slog.String("account_id", sess.AccountID))
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// SessionFrom returns the session stored by RequireSession.
func SessionFrom(ctx context.Context) (domain.Session, bool) {
	sess, ok := ctx.Value(sessionKey{}).(domain.Session)
	return sess, ok
}

// BearerToken extracts the token of an "Authorization: Bearer <token>" header.
func BearerToken(r *http.Request) (string, bool) {
	scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	token = strings.TrimSpace(token)
	return token, token != ""
}
