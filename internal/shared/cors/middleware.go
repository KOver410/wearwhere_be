package cors

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	allowHeaders = "Authorization, Content-Type"
	allowMethods = "GET, POST, PATCH, DELETE, OPTIONS"
)

func Middleware(allowedOrigins []string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		allowed[origin] = struct{}{}
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		_, isAllowed := allowed[origin]
		headers := c.Writer.Header()

		addVary(headers, "Origin")

		if isAllowed {
			headers.Set("Access-Control-Allow-Origin", origin)
			headers.Set("Access-Control-Allow-Headers", allowHeaders)
			headers.Set("Access-Control-Allow-Methods", allowMethods)
		}

		isPreflight := c.Request.Method == http.MethodOptions &&
			origin != "" &&
			c.GetHeader("Access-Control-Request-Method") != ""
		if isPreflight {
			if isAllowed {
				c.AbortWithStatus(http.StatusNoContent)
				return
			}
			c.AbortWithStatus(http.StatusForbidden)
			return
		}

		c.Next()
	}
}

func addVary(headers http.Header, value string) {
	for _, existing := range headers.Values("Vary") {
		for _, token := range strings.Split(existing, ",") {
			if strings.EqualFold(strings.TrimSpace(token), value) {
				return
			}
		}
	}
	headers.Add("Vary", value)
}
