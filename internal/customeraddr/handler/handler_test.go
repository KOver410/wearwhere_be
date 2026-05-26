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
	"github.com/wearwhere/wearwhere_be/internal/customeraddr/domain"
	"github.com/wearwhere/wearwhere_be/internal/customeraddr/handler"
	"github.com/wearwhere/wearwhere_be/internal/customeraddr/service"
	authvalidator "github.com/wearwhere/wearwhere_be/internal/shared/validator"
)

type stubRepo struct {
	listReturns []*domain.CustomerAddress
}

func (s *stubRepo) List(_ context.Context, _ uuid.UUID) ([]*domain.CustomerAddress, error) {
	return s.listReturns, nil
}
func (s *stubRepo) FindByID(_ context.Context, id, _ uuid.UUID) (*domain.CustomerAddress, error) {
	return &domain.CustomerAddress{ID: id, Label: "Nhà"}, nil
}
func (s *stubRepo) Create(_ context.Context, _ uuid.UUID, req *domain.CreateAddressRequest) (*domain.CustomerAddress, error) {
	return &domain.CustomerAddress{ID: uuid.New(), Label: req.Label, IsDefault: true}, nil
}
func (s *stubRepo) Update(_ context.Context, id, _ uuid.UUID, _ *domain.UpdateAddressRequest) (*domain.CustomerAddress, error) {
	return &domain.CustomerAddress{ID: id, Label: "Updated"}, nil
}
func (s *stubRepo) SoftDelete(_ context.Context, _, _ uuid.UUID) error { return nil }

func setupRouter(t *testing.T) (*gin.Engine, uuid.UUID) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	// Register custom binding tags (e164, etc.) on Gin's binding validator so
	// DTO `binding:"...,e164"` tags actually run when c.ShouldBindJSON is called.
	authvalidator.RegisterWithGin()
	r := gin.New()
	h := handler.New(service.New(&stubRepo{}))
	uid := uuid.New()
	rg := r.Group("/me", func(c *gin.Context) {
		authmw.SetUserIDForTest(c, uid)
		c.Next()
	})
	handler.Mount(rg, h)
	return r, uid
}

func TestCreate_201AndDefault(t *testing.T) {
	r, _ := setupRouter(t)
	body, _ := json.Marshal(domain.CreateAddressRequest{
		Label:          "Nhà",
		RecipientName:  "Nguyen Van X",
		RecipientPhone: "+84901234567",
		AddressLine:    "1 A",
		Ward:           "P 1",
		District:       "Q 1",
		City:           "TP HCM",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/me/addresses", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code, "body=%s", w.Body.String())
	var resp domain.AddressResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.IsDefault)
}

func TestDelete_204(t *testing.T) {
	r, _ := setupRouter(t)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/me/addresses/"+uuid.New().String(), nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusNoContent, w.Code)
}
