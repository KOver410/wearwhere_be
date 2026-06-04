package httpmw

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func newEngine(origins []string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CORS(origins))
	r.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "pong") })
	return r
}

func TestCORS_EmptyOriginsAllowsAny(t *testing.T) {
	r := newEngine(nil)
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "https://anything.test")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got == "" {
		t.Fatal("empty origin list should allow all origins, got no ACAO header")
	}
}

func TestCORS_AllowedOriginGetsHeader(t *testing.T) {
	r := newEngine([]string{"https://app.example.com"})
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "https://app.example.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Fatalf("Allow-Origin = %q, want %q", got, "https://app.example.com")
	}
}

func TestCORS_PreflightReturns204(t *testing.T) {
	r := newEngine([]string{"https://app.example.com"})
	req := httptest.NewRequest(http.MethodOptions, "/ping", nil)
	req.Header.Set("Origin", "https://app.example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want 204", w.Code)
	}
}

func TestCORS_DisallowedOriginNoEcho(t *testing.T) {
	r := newEngine([]string{"https://app.example.com"})
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got == "https://evil.example.com" {
		t.Fatalf("must not echo disallowed origin, got %q", got)
	}
}
