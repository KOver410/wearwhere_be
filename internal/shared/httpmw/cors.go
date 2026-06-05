// Package httpmw holds cross-cutting HTTP middleware shared across modules.
package httpmw

import (
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// CORS builds a gin middleware that allows the given browser origins.
//
// We use Bearer tokens (not cookies), so AllowCredentials stays false.
// When allowedOrigins is empty, all origins are allowed — a dev convenience;
// production MUST set CORS_ALLOWED_ORIGINS (enforced by the cutover checklist).
func CORS(allowedOrigins []string) gin.HandlerFunc {
	c := cors.Config{
		AllowMethods:     []string{"GET", "POST", "PATCH", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "Accept"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: false,
		MaxAge:           12 * time.Hour,
	}
	if len(allowedOrigins) == 0 {
		c.AllowAllOrigins = true
	} else {
		c.AllowOrigins = allowedOrigins
	}
	return cors.New(c)
}
