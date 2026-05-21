package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/cart/domain"
	"github.com/wearwhere/wearwhere_be/internal/cart/repo"
	"github.com/wearwhere/wearwhere_be/internal/cart/service"
	productdomain "github.com/wearwhere/wearwhere_be/internal/product/domain"
	productrepo "github.com/wearwhere/wearwhere_be/internal/product/repo"
)

type fakeCart struct {
	upsertReturns *domain.CartItem
	upsertErr     error
	findByVariant *domain.CartItem
	findVarErr    error
	findByID      *domain.CartItem
	findIDErr     error
	updateReturns *domain.CartItem
	deleteErr     error
}

func (f *fakeCart) UpsertAdd(_ context.Context, _, _ uuid.UUID, _ int, _ float64) (*domain.CartItem, error) {
	return f.upsertReturns, f.upsertErr
}
func (f *fakeCart) FindByID(_ context.Context, _, _ uuid.UUID) (*domain.CartItem, error) {
	return f.findByID, f.findIDErr
}
func (f *fakeCart) FindByVariant(_ context.Context, _, _ uuid.UUID) (*domain.CartItem, error) {
	return f.findByVariant, f.findVarErr
}
func (f *fakeCart) UpdateQty(_ context.Context, _, _ uuid.UUID, _ int, _ float64) (*domain.CartItem, error) {
	return f.updateReturns, nil
}
func (f *fakeCart) Delete(_ context.Context, _, _ uuid.UUID) error { return f.deleteErr }
func (f *fakeCart) Clear(_ context.Context, _ uuid.UUID) error     { return nil }
func (f *fakeCart) ListView(_ context.Context, _ uuid.UUID) ([]*domain.CartItemView, error) {
	return nil, nil
}

type fakeVariant struct {
	v   *productdomain.Variant
	p   *productdomain.Product
	err error
}

func (f *fakeVariant) Create(_ context.Context, _ uuid.UUID, _ *productdomain.CreateVariantRequest) (*productdomain.Variant, error) {
	return nil, nil
}
func (f *fakeVariant) FindByID(_ context.Context, _, _ uuid.UUID) (*productdomain.Variant, error) {
	return f.v, f.err
}
func (f *fakeVariant) ListByProduct(_ context.Context, _ uuid.UUID, _ bool) ([]*productdomain.Variant, error) {
	return nil, nil
}
func (f *fakeVariant) Update(_ context.Context, _, _ uuid.UUID, _ *productdomain.UpdateVariantRequest) (*productdomain.Variant, error) {
	return nil, nil
}
func (f *fakeVariant) SoftDelete(_ context.Context, _, _ uuid.UUID) error { return nil }
func (f *fakeVariant) FindForPurchase(_ context.Context, _ uuid.UUID) (*productdomain.Variant, *productdomain.Product, error) {
	return f.v, f.p, f.err
}

func TestAdd_QtyExceedsMax(t *testing.T) {
	s := service.New(&fakeCart{}, &fakeVariant{})
	_, err := s.Add(context.Background(), uuid.New(), uuid.New(), 11)
	require.ErrorIs(t, err, domain.ErrQtyExceedsMax)
}

func TestAdd_UnavailableVariant(t *testing.T) {
	s := service.New(&fakeCart{}, &fakeVariant{err: productrepo.ErrNotFound})
	_, err := s.Add(context.Background(), uuid.New(), uuid.New(), 2)
	require.ErrorIs(t, err, domain.ErrVariantUnavailable)
}

func TestAdd_OutOfStock(t *testing.T) {
	v := &productdomain.Variant{StockQty: 1, Price: 100}
	s := service.New(&fakeCart{findVarErr: repo.ErrNotFound}, &fakeVariant{v: v})
	_, err := s.Add(context.Background(), uuid.New(), uuid.New(), 2)
	require.ErrorIs(t, err, domain.ErrOutOfStock)
}

func TestAdd_CumulativeOverTen(t *testing.T) {
	v := &productdomain.Variant{StockQty: 100, Price: 100}
	existing := &domain.CartItem{Qty: 8}
	s := service.New(&fakeCart{findByVariant: existing}, &fakeVariant{v: v})
	_, err := s.Add(context.Background(), uuid.New(), uuid.New(), 5)
	require.ErrorIs(t, err, domain.ErrQtyExceedsMax)
}

func TestAdd_HappyPathReturnsUpsertResult(t *testing.T) {
	v := &productdomain.Variant{StockQty: 100, Price: 199000}
	out := &domain.CartItem{Qty: 3, PriceSnapshot: 199000}
	s := service.New(&fakeCart{
		findVarErr:    repo.ErrNotFound, // no existing row
		upsertReturns: out,
	}, &fakeVariant{v: v})
	got, err := s.Add(context.Background(), uuid.New(), uuid.New(), 3)
	require.NoError(t, err)
	require.Equal(t, 3, got.Qty)
}

func TestUpdateQty_RefreshesSnapshot(t *testing.T) {
	v := &productdomain.Variant{StockQty: 50, Price: 189000} // current price differs
	existing := &domain.CartItem{ID: uuid.New(), VariantID: uuid.New(), Qty: 2, PriceSnapshot: 199000}
	out := &domain.CartItem{Qty: 4, PriceSnapshot: 189000}
	s := service.New(&fakeCart{findByID: existing, updateReturns: out}, &fakeVariant{v: v})
	got, err := s.UpdateQty(context.Background(), existing.ID, uuid.New(), 4)
	require.NoError(t, err)
	require.Equal(t, 189000.0, got.PriceSnapshot)
}
