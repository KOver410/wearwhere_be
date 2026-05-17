package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	authmw "github.com/wearwhere/wearwhere_be/internal/auth/middleware"
	"github.com/wearwhere/wearwhere_be/internal/brand/domain"
	"github.com/wearwhere/wearwhere_be/internal/brand/repo"
)

// fakeBrandRepo: just enough to test middleware decisions.
type fakeRepo struct {
	brand *domain.Brand
	err   error
}

func (f *fakeRepo) FindByOwnerUserID(ctx context.Context, id uuid.UUID) (*domain.Brand, error) {
	return f.brand, f.err
}

// other methods unused
func (f *fakeRepo) FindByID(ctx context.Context, id uuid.UUID) (*domain.Brand, error)        { return nil, nil }
func (f *fakeRepo) FindBySlug(ctx context.Context, slug string) (*domain.Brand, error)        { return nil, nil }
func (f *fakeRepo) Update(ctx context.Context, id uuid.UUID, r *domain.UpdateBrandRequest) error { return nil }
func (f *fakeRepo) List(ctx context.Context, q, sort string, l, o int) ([]*domain.Brand, int, error) {
	return nil, 0, nil
}

func setup(brandRepo repo.BrandRepo, userID uuid.UUID) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		// simulate RequireAuth populating user_id
		authmw.SetUserIDForTest(c, userID)
		c.Next()
	})
	r.Use(BrandContext(brandRepo))
	r.GET("/x", func(c *gin.Context) {
		bid, _ := c.Get("brand.id")
		c.JSON(http.StatusOK, gin.H{"brand_id": bid})
	})
	return r
}

func TestBrandContext_NoBrand_403(t *testing.T) {
	r := setup(&fakeRepo{err: repo.ErrNotFound}, uuid.New())
	rec := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/x", nil)
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), "NO_BRAND_OWNED")
}

func TestBrandContext_Suspended_403(t *testing.T) {
	bid := uuid.New()
	uid := uuid.New()
	r := setup(&fakeRepo{brand: &domain.Brand{ID: bid, OwnerUserID: uid, Status: domain.BrandStatusSuspended}}, uid)
	rec := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/x", nil)
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), "BRAND_SUSPENDED")
}

func TestBrandContext_Active_PassesThrough(t *testing.T) {
	bid := uuid.New()
	uid := uuid.New()
	r := setup(&fakeRepo{brand: &domain.Brand{ID: bid, OwnerUserID: uid, Status: domain.BrandStatusActive}}, uid)
	rec := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/x", nil)
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), bid.String())
}
