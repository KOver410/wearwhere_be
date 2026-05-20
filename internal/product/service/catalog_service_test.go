package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/product/domain"
	"github.com/wearwhere/wearwhere_be/internal/product/repo"
)

type fakeCatalogRepo struct {
	items         []*domain.CatalogItem
	total         int
	err           error
	suggestions   []string
	detailErr     error
	detailProduct *domain.Product
	detailByIDErr error
}

func (f *fakeCatalogRepo) List(ctx context.Context, q *domain.ListProductsQuery) ([]*domain.CatalogItem, int, error) {
	return f.items, f.total, f.err
}
func (f *fakeCatalogRepo) Detail(ctx context.Context, bs, ps string) (*domain.Product, *domain.Category, []*domain.Variant, []*domain.Image, []*domain.StyleTag, error) {
	if f.detailErr != nil {
		return nil, nil, nil, nil, nil, f.detailErr
	}
	return f.detailProduct, nil, nil, nil, nil, nil
}
func (f *fakeCatalogRepo) DetailByID(ctx context.Context, id uuid.UUID) (*domain.Product, *domain.Category, []*domain.Variant, []*domain.Image, []*domain.StyleTag, error) {
	if f.detailByIDErr != nil {
		return nil, nil, nil, nil, nil, f.detailByIDErr
	}
	return f.detailProduct, nil, nil, nil, nil, nil
}
func (f *fakeCatalogRepo) Suggestions(ctx context.Context, q string, limit int) ([]string, error) {
	return f.suggestions, nil
}

type fakeProductRepoNoOp struct{ viewCount int32 }

func (f *fakeProductRepoNoOp) IncrementViewCount(ctx context.Context, id uuid.UUID) error {
	atomic.AddInt32(&f.viewCount, 1)
	return nil
}

// rest unused — satisfy interface with errors
func (f *fakeProductRepoNoOp) Create(ctx context.Context, brandID uuid.UUID, slug string, req *domain.CreateProductRequest) (*domain.Product, error) {
	return nil, errors.New("nope")
}
func (f *fakeProductRepoNoOp) SlugExists(ctx context.Context, brandID uuid.UUID, slug string) (bool, error) {
	return false, nil
}
func (f *fakeProductRepoNoOp) FindByID(ctx context.Context, id uuid.UUID) (*domain.Product, error) {
	return nil, repo.ErrNotFound
}
func (f *fakeProductRepoNoOp) FindByBrandSlug(ctx context.Context, bs, ps string) (*domain.Product, error) {
	return nil, repo.ErrNotFound
}
func (f *fakeProductRepoNoOp) Update(ctx context.Context, id, brandID uuid.UUID, r *domain.UpdateProductRequest) error {
	return nil
}
func (f *fakeProductRepoNoOp) SoftDelete(ctx context.Context, id, brandID uuid.UUID) error { return nil }
func (f *fakeProductRepoNoOp) ListByBrand(ctx context.Context, brandID uuid.UUID, l, o int) ([]*domain.Product, int, error) {
	return nil, 0, nil
}
func (f *fakeProductRepoNoOp) SetStyleTags(ctx context.Context, p uuid.UUID, ids []uuid.UUID) error {
	return nil
}
func (f *fakeProductRepoNoOp) GetStyleTags(ctx context.Context, p uuid.UUID) ([]*domain.StyleTag, error) {
	return nil, nil
}

func TestCatalog_List_EmptyResults_ReturnsSuggestions(t *testing.T) {
	cr := &fakeCatalogRepo{items: nil, total: 0, suggestions: []string{"Áo Thun"}}
	svc := NewCatalog(cr, &fakeProductRepoNoOp{})
	items, total, suggestions, err := svc.List(context.Background(), &domain.ListProductsQuery{
		Q: "asdfgh", Page: 1, Limit: 10,
	})
	require.NoError(t, err)
	require.Len(t, items, 0)
	require.Equal(t, 0, total)
	require.Equal(t, []string{"Áo Thun"}, suggestions)
}

func TestCatalog_List_NoQuery_NoSuggestions(t *testing.T) {
	cr := &fakeCatalogRepo{items: nil, total: 0, suggestions: []string{"x"}}
	svc := NewCatalog(cr, &fakeProductRepoNoOp{})
	_, _, suggestions, _ := svc.List(context.Background(), &domain.ListProductsQuery{
		Q: "", Page: 1, Limit: 10,
	})
	require.Nil(t, suggestions)
}

func TestCatalog_Detail_NotFound_ReturnsErrProductNotFound(t *testing.T) {
	cr := &fakeCatalogRepo{detailErr: repo.ErrNotFound}
	svc := NewCatalog(cr, &fakeProductRepoNoOp{})
	_, _, _, _, _, err := svc.Detail(context.Background(), "brand-slug", "product-slug")
	require.True(t, errors.Is(err, domain.ErrProductNotFound), "expected ErrProductNotFound, got %v", err)
}

func TestCatalog_DetailByID_NotFound_ReturnsErrProductNotFound(t *testing.T) {
	cr := &fakeCatalogRepo{detailByIDErr: repo.ErrNotFound}
	svc := NewCatalog(cr, &fakeProductRepoNoOp{})
	_, _, _, _, _, err := svc.DetailByID(context.Background(), uuid.New())
	require.True(t, errors.Is(err, domain.ErrProductNotFound), "expected ErrProductNotFound, got %v", err)
}

func TestCatalog_Detail_FiresViewCountIncrement(t *testing.T) {
	prod := &domain.Product{ID: uuid.New()}
	cr := &fakeCatalogRepo{detailProduct: prod}
	fp := &fakeProductRepoNoOp{}
	svc := NewCatalog(cr, fp)
	_, _, _, _, _, err := svc.Detail(context.Background(), "brand-slug", "product-slug")
	require.NoError(t, err)
	// Poll until the goroutine increments the counter or we time out.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && atomic.LoadInt32(&fp.viewCount) == 0 {
		time.Sleep(time.Millisecond)
	}
	require.Equal(t, int32(1), atomic.LoadInt32(&fp.viewCount), "view-count goroutine did not fire within 2 s")
}
