package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	authmw "github.com/wearwhere/wearwhere_be/internal/auth/middleware"
	"github.com/wearwhere/wearwhere_be/internal/recommendation/domain"
	"github.com/wearwhere/wearwhere_be/internal/recommendation/handler"
)

type fakeSvc struct{ last int }

func (f *fakeSvc) Recommend(_ context.Context, _ uuid.UUID, limit int) (*domain.RecommendationsResponse, error) {
	f.last = limit
	return &domain.RecommendationsResponse{
		Items:  []domain.RecProductCard{{ID: uuid.New().String(), Name: "X"}},
		Source: "trending", OnboardingPrompt: true,
	}, nil
}

func setup(f *fakeSvc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		authmw.SetUserIDForTest(c, uuid.New())
		c.Next()
	})
	handler.Mount(r.Group("/me"), handler.New(f))
	return r
}

func TestList_ReturnsFeed(t *testing.T) {
	f := &fakeSvc{}
	r := setup(f)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/me/recommendations?limit=12", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, 12, f.last, "limit query must be parsed and forwarded")

	var body domain.RecommendationsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "trending", body.Source)
	require.True(t, body.OnboardingPrompt)
	require.Len(t, body.Items, 1)
}

func TestList_NoLimitForwardsZero(t *testing.T) {
	f := &fakeSvc{last: -1}
	r := setup(f)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/me/recommendations", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, 0, f.last, "missing limit forwards 0 (service applies default)")
}
