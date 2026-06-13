package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	authmw "github.com/wearwhere/wearwhere_be/internal/auth/middleware"
	"github.com/wearwhere/wearwhere_be/internal/review/domain"
	"github.com/wearwhere/wearwhere_be/internal/review/repo"
	"github.com/wearwhere/wearwhere_be/internal/review/service"
)

type reviewFake struct {
	productExists bool
	delivered     bool
}

func (f *reviewFake) ProductExists(context.Context, uuid.UUID) (bool, error) { return f.productExists, nil }
func (f *reviewFake) HasDeliveredPurchase(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return f.delivered, nil
}
func (f *reviewFake) Create(_ context.Context, r *domain.Review) error { r.ID = uuid.New(); return nil }
func (f *reviewFake) GetByID(context.Context, uuid.UUID) (*domain.Review, error) {
	return nil, repo.ErrNotFound
}
func (f *reviewFake) Update(context.Context, uuid.UUID, int, string, *string) error { return nil }
func (f *reviewFake) SoftDelete(context.Context, uuid.UUID) error                   { return nil }
func (f *reviewFake) ListByProduct(context.Context, uuid.UUID, repo.ListFilter) ([]*domain.ReviewView, int, error) {
	return nil, 0, nil
}
func (f *reviewFake) Aggregate(context.Context, uuid.UUID) (repo.Aggregate, error) {
	return repo.Aggregate{}, nil
}

func setup(f repo.Repo, userID uuid.UUID) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := New(service.NewWithRepo(f))
	v1 := r.Group("/api/v1")
	MountReviewsPublic(v1, h)
	authed := v1.Group("", func(c *gin.Context) { authmw.SetUserIDForTest(c, userID); c.Next() })
	MountReviewsAuthed(authed, h)
	return r
}

func TestCreate_InvalidBody_400(t *testing.T) {
	r := setup(&reviewFake{productExists: true, delivered: true}, uuid.New())
	w := httptest.NewRecorder()
	payload, _ := json.Marshal(map[string]any{"rating": 5, "body": "too short"})
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/products/"+uuid.New().String()+"/reviews", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestCreate_Valid_201(t *testing.T) {
	r := setup(&reviewFake{productExists: true, delivered: true}, uuid.New())
	w := httptest.NewRecorder()
	payload, _ := json.Marshal(map[string]any{"rating": 5, "body": "This is a valid review body over twenty chars", "fit": "true"})
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/products/"+uuid.New().String()+"/reviews", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}
}
