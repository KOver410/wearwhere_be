package handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	authmw "github.com/wearwhere/wearwhere_be/internal/auth/middleware"
	"github.com/wearwhere/wearwhere_be/internal/cart/handler"
)

func TestCartRoutes_DeleteWithBadUUID_404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := handler.New(nil)
	rg := r.Group("/me", func(c *gin.Context) { authmw.SetUserIDForTest(c, uuid.New()); c.Next() })
	handler.Mount(rg, h)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/me/cart/items/not-a-uuid", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)
}
