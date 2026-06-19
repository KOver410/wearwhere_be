package handler_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/promo/handler"
)

// These tests exercise only the short-circuit paths (bad UUID / bind error)
// that return before the service is touched, so a nil service is safe.

func TestAdminPromo_BadUUID_Get404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	handler.MountAdmin(r.Group("/admin"), handler.New(nil))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/admin/promo-codes/not-a-uuid", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestAdminPromo_BadUUID_Patch404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	handler.MountAdmin(r.Group("/admin"), handler.New(nil))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PATCH", "/admin/promo-codes/not-a-uuid",
		bytes.NewBufferString(`{"is_active":false}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestAdminPromo_Create_InvalidBody_400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	handler.MountAdmin(r.Group("/admin"), handler.New(nil))

	w := httptest.NewRecorder()
	// Missing required code/discount_type → binding error before service call.
	req, _ := http.NewRequest("POST", "/admin/promo-codes",
		bytes.NewBufferString(`{"discount_value":10}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}
