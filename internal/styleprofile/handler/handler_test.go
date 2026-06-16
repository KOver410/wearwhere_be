package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	authmw "github.com/wearwhere/wearwhere_be/internal/auth/middleware"
	"github.com/wearwhere/wearwhere_be/internal/styleprofile/domain"
	"github.com/wearwhere/wearwhere_be/internal/styleprofile/handler"
	"github.com/wearwhere/wearwhere_be/internal/styleprofile/repo"
	"github.com/wearwhere/wearwhere_be/internal/styleprofile/service"
)

type fakeRepo struct{ unknown []uuid.UUID }

func (f *fakeRepo) Load(_ context.Context, _ uuid.UUID) (*domain.StyleProfileView, error) {
	return nil, repo.ErrNotFound
}
func (f *fakeRepo) Upsert(_ context.Context, p domain.UpsertParams) (*domain.StyleProfileView, error) {
	return &domain.StyleProfileView{UserID: p.UserID, BudgetMin: p.BudgetMin}, nil
}
func (f *fakeRepo) UnknownTagIDs(_ context.Context, _ []uuid.UUID) ([]uuid.UUID, error) {
	return f.unknown, nil
}

func setup(f *fakeRepo) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := handler.New(service.New(f))
	r := gin.New()
	rg := r.Group("/me", func(c *gin.Context) {
		authmw.SetUserIDForTest(c, uuid.New())
		c.Next()
	})
	handler.Mount(rg, h)
	return r
}

func TestGet_EmptyProfile200(t *testing.T) {
	r := setup(&fakeRepo{})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/me/style-profile", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var body domain.StyleProfileResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, []domain.StyleTagRef{}, body.StyleTags)
}

func TestPut_UnknownTag400WithDetails(t *testing.T) {
	bad := uuid.New()
	r := setup(&fakeRepo{unknown: []uuid.UUID{bad}})
	w := httptest.NewRecorder()
	payload, _ := json.Marshal(map[string]any{"style_tag_ids": []string{bad.String()}})
	req, _ := http.NewRequest(http.MethodPut, "/me/style-profile", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "VALIDATION_FAILED")
	require.Contains(t, w.Body.String(), bad.String())
}

func TestPut_BadBudget400(t *testing.T) {
	r := setup(&fakeRepo{})
	w := httptest.NewRecorder()
	payload, _ := json.Marshal(map[string]any{"budget_min": 500000, "budget_max": 100000})
	req, _ := http.NewRequest(http.MethodPut, "/me/style-profile", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}
