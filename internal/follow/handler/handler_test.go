package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	authmw "github.com/wearwhere/wearwhere_be/internal/auth/middleware"
	"github.com/wearwhere/wearwhere_be/internal/follow/domain"
	"github.com/wearwhere/wearwhere_be/internal/follow/service"
)

type fakeRepo struct{ userExists bool }

func (f *fakeRepo) UserExists(context.Context, uuid.UUID) (bool, error)  { return f.userExists, nil }
func (f *fakeRepo) BrandExists(context.Context, uuid.UUID) (bool, error) { return true, nil }
func (f *fakeRepo) FollowUser(context.Context, uuid.UUID, uuid.UUID) (int, error)    { return 1, nil }
func (f *fakeRepo) UnfollowUser(context.Context, uuid.UUID, uuid.UUID) (int, error)  { return 0, nil }
func (f *fakeRepo) FollowBrand(context.Context, uuid.UUID, uuid.UUID) (int, error)   { return 1, nil }
func (f *fakeRepo) UnfollowBrand(context.Context, uuid.UUID, uuid.UUID) (int, error) { return 0, nil }
func (f *fakeRepo) ListFollowingUsers(context.Context, uuid.UUID, int, int) ([]domain.FollowingUserItem, int, error) {
	return nil, 0, nil
}
func (f *fakeRepo) ListFollowingBrands(context.Context, uuid.UUID, int, int) ([]domain.FollowingBrandItem, int, error) {
	return nil, 0, nil
}

func setup(userID uuid.UUID, userExists bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := New(service.New(&fakeRepo{userExists: userExists}))
	g := r.Group("/api/v1", func(c *gin.Context) { authmw.SetUserIDForTest(c, userID); c.Next() })
	MountFollowAuthed(g, h)
	return r
}

func TestFollowUser_Self_400(t *testing.T) {
	id := uuid.New()
	r := setup(id, true)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/users/"+id.String()+"/follow", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 (self-follow)", w.Code)
	}
}

func TestFollowUser_OK(t *testing.T) {
	r := setup(uuid.New(), true)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/users/"+uuid.New().String()+"/follow", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", w.Code, w.Body.String())
	}
}
