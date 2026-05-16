package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/wearwhere/wearwhere_be/internal/auth/domain"
	"github.com/wearwhere/wearwhere_be/pkg/httpx"
	jwtsvc "github.com/wearwhere/wearwhere_be/internal/shared/jwt"
)

// RequireAuth parses the Bearer token, verifies it, and attaches the claims
// to the request context. Aborts with 401 on any failure.
func RequireAuth(issuer *jwtsvc.Issuer) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			httpx.ErrorFromApp(c, domain.ErrUnauthorized)
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		if token == "" {
			httpx.ErrorFromApp(c, domain.ErrUnauthorized)
			return
		}
		claims, err := issuer.Verify(token)
		if err != nil {
			httpx.ErrorFromApp(c, domain.ErrUnauthorized)
			return
		}
		uid, err := uuid.Parse(claims.UserID)
		if err != nil {
			httpx.ErrorFromApp(c, domain.ErrUnauthorized)
			return
		}
		setAuthCtx(c, uid, claims.Role, claims.Email)
		c.Next()
	}
}

// OptionalAuth populates the auth context if the request has a valid token
// but does NOT 401 on missing / invalid token. Useful for endpoints that
// behave differently for guests vs. logged-in users.
func OptionalAuth(issuer *jwtsvc.Issuer) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			c.Next()
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		claims, err := issuer.Verify(token)
		if err != nil {
			c.Next()
			return
		}
		if uid, err := uuid.Parse(claims.UserID); err == nil {
			setAuthCtx(c, uid, claims.Role, claims.Email)
		}
		c.Next()
	}
}
