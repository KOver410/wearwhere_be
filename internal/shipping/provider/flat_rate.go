package provider

import (
	"context"

	brandrepo "github.com/wearwhere/wearwhere_be/internal/brand/repo"
	shippingdomain "github.com/wearwhere/wearwhere_be/internal/shipping/domain"
)

// FlatRateProvider reads brands.shipping_flat_fee_vnd (DB column).
type FlatRateProvider struct {
	brandRepo brandrepo.BrandRepo
}

func NewFlatRateProvider(b brandrepo.BrandRepo) *FlatRateProvider {
	return &FlatRateProvider{brandRepo: b}
}

func (p *FlatRateProvider) Calculate(ctx context.Context, r CalcReq) (*shippingdomain.FeeQuote, error) {
	b, err := p.brandRepo.FindByID(ctx, r.BrandID)
	if err != nil {
		return nil, err
	}
	return &shippingdomain.FeeQuote{
		AmountVND: b.ShippingFlatFeeVND,
		Currency:  "VND",
	}, nil
}
