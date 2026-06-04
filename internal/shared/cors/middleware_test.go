package cors

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const allowedOrigin = "http://localhost:5173"

func TestMiddlewareAllowedPreflight(t *testing.T) {
	response := performRequest(t, http.MethodOptions, allowedOrigin, "POST", false)

	require.Equal(t, http.StatusNoContent, response.Code)
	require.Equal(t, allowedOrigin, response.Header().Get("Access-Control-Allow-Origin"))
	require.Equal(t, "Authorization, Content-Type", response.Header().Get("Access-Control-Allow-Headers"))
	require.Equal(t, "GET, POST, PATCH, DELETE, OPTIONS", response.Header().Get("Access-Control-Allow-Methods"))
	require.Equal(t, "Origin", response.Header().Get("Vary"))
	require.Empty(t, response.Header().Get("Access-Control-Allow-Credentials"))
}

func TestMiddlewareRejectsUnknownOriginPreflight(t *testing.T) {
	response := performRequest(t, http.MethodOptions, "https://unknown.example", "POST", false)

	require.Equal(t, http.StatusForbidden, response.Code)
	require.Empty(t, response.Header().Get("Access-Control-Allow-Origin"))
	require.Equal(t, []string{"Origin"}, response.Header().Values("Vary"))
}

func TestMiddlewareAllowsOrdinaryOptionsToContinue(t *testing.T) {
	response := performRequest(t, http.MethodOptions, allowedOrigin, "", false)

	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "ok", response.Body.String())
}

func TestMiddlewareAddsHeadersToAllowedNormalResponse(t *testing.T) {
	response := performRequest(t, http.MethodGet, allowedOrigin, "", false)

	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, allowedOrigin, response.Header().Get("Access-Control-Allow-Origin"))
	require.Equal(t, "Authorization, Content-Type", response.Header().Get("Access-Control-Allow-Headers"))
	require.Equal(t, "GET, POST, PATCH, DELETE, OPTIONS", response.Header().Get("Access-Control-Allow-Methods"))
	require.Equal(t, "Origin", response.Header().Get("Vary"))
	require.Empty(t, response.Header().Get("Access-Control-Allow-Credentials"))
}

func TestMiddlewareAllowsUnknownOriginNormalResponseWithoutCORSHeaders(t *testing.T) {
	response := performRequest(t, http.MethodGet, "https://unknown.example", "", false)

	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "ok", response.Body.String())
	require.Empty(t, response.Header().Get("Access-Control-Allow-Origin"))
	require.Equal(t, []string{"Origin"}, response.Header().Values("Vary"))
}

func TestMiddlewareAddsVaryForAbsentOrigin(t *testing.T) {
	response := performRequest(t, http.MethodGet, "", "", false)

	require.Equal(t, http.StatusOK, response.Code)
	require.Empty(t, response.Header().Get("Access-Control-Allow-Origin"))
	require.Equal(t, []string{"Origin"}, response.Header().Values("Vary"))
}

func TestMiddlewarePreservesExistingVaryWithoutDuplicatingOrigin(t *testing.T) {
	response := performRequest(t, http.MethodGet, allowedOrigin, "", true)

	require.Equal(t, http.StatusOK, response.Code)
	require.ElementsMatch(t, []string{"Accept-Encoding", "Origin"}, response.Header().Values("Vary"))
}

func performRequest(t *testing.T, method, origin, requestedMethod string, addExistingVary bool) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	if addExistingVary {
		router.Use(func(c *gin.Context) {
			c.Header("Vary", "Accept-Encoding")
			c.Writer.Header().Add("Vary", "Origin")
			c.Next()
		})
	}
	router.Use(Middleware([]string{allowedOrigin}))
	router.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	router.OPTIONS("/", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	request := httptest.NewRequest(method, "/", nil)
	if origin != "" {
		request.Header.Set("Origin", origin)
	}
	if requestedMethod != "" {
		request.Header.Set("Access-Control-Request-Method", requestedMethod)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
