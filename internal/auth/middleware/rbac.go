package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/wearwhere/wearwhere_be/internal/auth/domain"
	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

// RequireRole rejects requests whose authenticated user's role is not in the
// allowed list. Must be chained AFTER RequireAuth.
func RequireRole(roles ...domain.Role) gin.HandlerFunc {
	return func(c *gin.Context) {
		current := Role(c)
		for _, r := range roles {
			if r == current {
				c.Next()
				return
			}
		}
		httpx.ErrorFromApp(c, domain.ErrForbidden)
	}
}
