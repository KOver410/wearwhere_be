package handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	authmw "github.com/wearwhere/wearwhere_be/internal/auth/middleware"
	"github.com/wearwhere/wearwhere_be/internal/wishlist/handler"
)

func TestWishlistRoutes_RemoveIsIdempotent204(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := handler.New(nil) // svc nil; only the bad-uuid short-circuit is exercised
	rg := r.Group("/me", func(c *gin.Context) { authmw.SetUserIDForTest(c, uuid.New()); c.Next() })
	handler.Mount(rg, h)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/me/wishlist/not-a-uuid", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusNoContent, w.Code)
}
