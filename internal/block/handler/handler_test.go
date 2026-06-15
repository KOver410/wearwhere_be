package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	authmw "github.com/wearwhere/wearwhere_be/internal/auth/middleware"
	"github.com/wearwhere/wearwhere_be/internal/block/domain"
	"github.com/wearwhere/wearwhere_be/internal/block/service"
)

type fakeRepo struct{ userExists bool }

func (f *fakeRepo) UserExists(context.Context, uuid.UUID) (bool, error)  { return f.userExists, nil }
func (f *fakeRepo) Block(context.Context, uuid.UUID, uuid.UUID) error    { return nil }
func (f *fakeRepo) Unblock(context.Context, uuid.UUID, uuid.UUID) error  { return nil }
func (f *fakeRepo) ListBlocked(context.Context, uuid.UUID, int, int) ([]domain.BlockedUserItem, int, error) {
	return nil, 0, nil
}

func setup(userID uuid.UUID, userExists bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := New(service.New(&fakeRepo{userExists: userExists}))
	g := r.Group("/api/v1", func(c *gin.Context) { authmw.SetUserIDForTest(c, userID); c.Next() })
	MountBlockAuthed(g, h)
	return r
}

func TestBlockUser_Self_400(t *testing.T) {
	id := uuid.New()
	r := setup(id, true)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/users/"+id.String()+"/block", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 (self-block)", w.Code)
	}
}

func TestBlockUser_OK(t *testing.T) {
	r := setup(uuid.New(), true)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/users/"+uuid.New().String()+"/block", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", w.Code, w.Body.String())
	}
}

func TestListBlocked_OK(t *testing.T) {
	r := setup(uuid.New(), true)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/me/blocks", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", w.Code, w.Body.String())
	}
}

func TestUnblockUser_OK(t *testing.T) {
	r := setup(uuid.New(), true)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/api/v1/users/"+uuid.New().String()+"/block", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", w.Code, w.Body.String())
	}
}

func TestUnblockUser_InvalidUUID(t *testing.T) {
	r := setup(uuid.New(), true)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/api/v1/users/not-a-uuid/block", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404 (matches the module's bad-UUID convention); body=%s", w.Code, w.Body.String())
	}
}
