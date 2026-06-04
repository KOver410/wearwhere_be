package provider_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	branddomain "github.com/wearwhere/wearwhere_be/internal/brand/domain"
	"github.com/wearwhere/wearwhere_be/internal/shipping/provider"
)

// fakeBrandRepo satisfies brandrepo.BrandRepo.
// Only FindByID is exercised; all other methods panic if called.
type fakeBrandRepo struct {
	byID map[uuid.UUID]*branddomain.Brand
	err  error
}

func (f *fakeBrandRepo) FindByID(_ context.Context, id uuid.UUID) (*branddomain.Brand, error) {
	if f.err != nil {
		return nil, f.err
	}
	if b, ok := f.byID[id]; ok {
		return b, nil
	}
	return nil, errors.New("not found")
}

func (f *fakeBrandRepo) FindBySlug(_ context.Context, _ string) (*branddomain.Brand, error) {
	panic("unused in flat rate tests")
}

func (f *fakeBrandRepo) FindByOwnerUserID(_ context.Context, _ uuid.UUID) (*branddomain.Brand, error) {
	panic("unused in flat rate tests")
}

func (f *fakeBrandRepo) Update(_ context.Context, _ uuid.UUID, _ *branddomain.UpdateBrandRequest) error {
	panic("unused in flat rate tests")
}

func (f *fakeBrandRepo) List(_ context.Context, _ string, _ string, _, _ int) ([]*branddomain.Brand, int, error) {
	panic("unused in flat rate tests")
}

func TestFlatRate_Quote_SingleOption(t *testing.T) {
	id := uuid.New()
	repo := &fakeBrandRepo{byID: map[uuid.UUID]*branddomain.Brand{
		id: {ID: id, ShippingFlatFeeVND: 30000},
	}}
	p := provider.NewFlatRateProvider(repo)

	opts, err := p.Quote(context.Background(), provider.CalcReq{BrandID: id})
	if err != nil {
		t.Fatalf("Quote: %v", err)
	}
	if len(opts) != 1 || opts[0].AmountVND != 30000 || opts[0].Carrier != "flat" {
		t.Fatalf("unexpected options: %+v", opts)
	}
}

func TestFactory_DefaultsToFlat(t *testing.T) {
	id := uuid.New()
	repo := &fakeBrandRepo{byID: map[uuid.UUID]*branddomain.Brand{
		id: {ID: id, ShippingFlatFeeVND: 30000},
	}}
	p, err := provider.NewFromConfig(provider.Config{Provider: ""}, repo, nil)
	require.NoError(t, err)
	opts, err := p.Quote(context.Background(), provider.CalcReq{BrandID: id})
	require.NoError(t, err)
	require.Equal(t, 1, len(opts))
	require.Equal(t, int64(30000), opts[0].AmountVND)
	require.Equal(t, "flat", opts[0].Carrier)
}

func TestFactory_UnknownProvider(t *testing.T) {
	_, err := provider.NewFromConfig(provider.Config{Provider: "ghn"}, nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown shipping provider")
}
