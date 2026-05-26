package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	productdomain "github.com/wearwhere/wearwhere_be/internal/product/domain"
	"github.com/wearwhere/wearwhere_be/internal/wishlist/domain"
	"github.com/wearwhere/wearwhere_be/internal/wishlist/service"
)

type fakeWishlist struct {
	addErr error
	rmErr  error
	has    map[uuid.UUID]bool
}

func (f *fakeWishlist) Add(_ context.Context, _, _ uuid.UUID) error    { return f.addErr }
func (f *fakeWishlist) Remove(_ context.Context, _, _ uuid.UUID) error { return f.rmErr }
func (f *fakeWishlist) List(_ context.Context, _ uuid.UUID, _, _ int) ([]*domain.WishlistItemView, int, error) {
	return nil, 0, nil
}
func (f *fakeWishlist) Contains(_ context.Context, _ uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]bool, error) {
	out := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		if f.has[id] {
			out[id] = true
		}
	}
	return out, nil
}

type fakeProductRepo struct {
	ret *productdomain.Product
	err error
}

func (f *fakeProductRepo) Create(_ context.Context, _ uuid.UUID, _ string, _ *productdomain.CreateProductRequest) (*productdomain.Product, error) {
	return nil, nil
}
func (f *fakeProductRepo) FindByID(_ context.Context, _ uuid.UUID) (*productdomain.Product, error) {
	return f.ret, f.err
}
func (f *fakeProductRepo) FindByBrandSlug(_ context.Context, _, _ string) (*productdomain.Product, error) {
	return nil, nil
}
func (f *fakeProductRepo) Update(_ context.Context, _, _ uuid.UUID, _ *productdomain.UpdateProductRequest) error {
	return nil
}
func (f *fakeProductRepo) SoftDelete(_ context.Context, _, _ uuid.UUID) error { return nil }
func (f *fakeProductRepo) ListByBrand(_ context.Context, _ uuid.UUID, _, _ int) ([]*productdomain.Product, int, error) {
	return nil, 0, nil
}
func (f *fakeProductRepo) SlugExists(_ context.Context, _ uuid.UUID, _ string) (bool, error) {
	return false, nil
}
func (f *fakeProductRepo) IncrementViewCount(_ context.Context, _ uuid.UUID) error { return nil }
func (f *fakeProductRepo) SetStyleTags(_ context.Context, _ uuid.UUID, _ []uuid.UUID) error {
	return nil
}
func (f *fakeProductRepo) GetStyleTags(_ context.Context, _ uuid.UUID) ([]*productdomain.StyleTag, error) {
	return nil, nil
}

func TestAdd_InactiveProductReturnsNotFound(t *testing.T) {
	inactive := &productdomain.Product{Status: "draft"}
	s := service.New(&fakeWishlist{}, &fakeProductRepo{ret: inactive})
	err := s.Add(context.Background(), uuid.New(), uuid.New())
	require.ErrorIs(t, err, domain.ErrProductNotFound)
}

func TestAdd_SoftDeletedProductReturnsNotFound(t *testing.T) {
	now := time.Now()
	deleted := &productdomain.Product{Status: "active", DeletedAt: &now}
	s := service.New(&fakeWishlist{}, &fakeProductRepo{ret: deleted})
	err := s.Add(context.Background(), uuid.New(), uuid.New())
	require.ErrorIs(t, err, domain.ErrProductNotFound)
}

func TestContains_AbsentIDsMapToFalse(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	s := service.New(&fakeWishlist{has: map[uuid.UUID]bool{a: true}}, &fakeProductRepo{})
	out, err := s.Contains(context.Background(), uuid.New(), []uuid.UUID{a, b})
	require.NoError(t, err)
	require.True(t, out[a])
	require.False(t, out[b])
}
