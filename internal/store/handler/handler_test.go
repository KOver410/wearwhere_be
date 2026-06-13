package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/wearwhere/wearwhere_be/internal/maps/goong"
	"github.com/wearwhere/wearwhere_be/internal/store/service"
)

func newTestRouter(svc *service.Service) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	MountStoresPublic(r.Group("/api/v1"), NewHandler(svc))
	return r
}

func TestNearby_MissingLatLng_400(t *testing.T) {
	svc := service.New(nil, goong.NewMockClient())
	r := newTestRouter(svc)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/stores/nearby", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if _, ok := body["error"]; !ok {
		t.Errorf("expected error envelope, got %s", w.Body.String())
	}
}

func TestDirections_BadFrom_400(t *testing.T) {
	svc := service.New(nil, goong.NewMockClient())
	r := newTestRouter(svc)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/stores/3f1e8c4a-0000-0000-0000-000000000000/directions", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (missing from)", w.Code)
	}
}
