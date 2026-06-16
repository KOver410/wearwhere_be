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
	"github.com/wearwhere/wearwhere_be/internal/wardrobe/domain"
	"github.com/wearwhere/wearwhere_be/internal/wardrobe/handler"
)

type fakeSvc struct{ regen bool }

func (f *fakeSvc) Get(_ context.Context, _ uuid.UUID) (*domain.WardrobeResponse, error) {
	return &domain.WardrobeResponse{OutfitsStatus: "ready", Outfits: []domain.Outfit{{Title: "L"}}}, nil
}
func (f *fakeSvc) Regenerate(_ context.Context, _ uuid.UUID) (*domain.WardrobeResponse, error) {
	f.regen = true
	return &domain.WardrobeResponse{OutfitsStatus: "ready"}, nil
}

func setup(f *fakeSvc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { authmw.SetUserIDForTest(c, uuid.New()); c.Next() })
	handler.Mount(r.Group("/me"), handler.New(f))
	return r
}

func TestGet_ReturnsWardrobe(t *testing.T) {
	r := setup(&fakeSvc{})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/me/wardrobe", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var body domain.WardrobeResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "ready", body.OutfitsStatus)
	require.Len(t, body.Outfits, 1)
}

func TestRegenerate_CallsService(t *testing.T) {
	f := &fakeSvc{}
	r := setup(f)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/me/wardrobe/regenerate", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.True(t, f.regen)
}
