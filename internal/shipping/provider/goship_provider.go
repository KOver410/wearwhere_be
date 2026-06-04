package provider

import (
	"context"
	"errors"

	"github.com/google/uuid"

	shippingdomain "github.com/wearwhere/wearwhere_be/internal/shipping/domain"
	"github.com/wearwhere/wearwhere_be/internal/shipping/goship"
	"github.com/wearwhere/wearwhere_be/internal/shipping/weight"
)

// ErrDestinationIncomplete is returned when the customer address lacks codes.
var ErrDestinationIncomplete = errors.New("shipping: destination missing city/district code")

// ErrPickupIncomplete is returned when the brand's pickup address lacks codes.
var ErrPickupIncomplete = errors.New("shipping: brand pickup address missing city/district code")

// PickupRepo returns the brand's primary pickup address location codes (Goship string codes).
type PickupRepo interface {
	PrimaryAddressCodes(ctx context.Context, brandID uuid.UUID) (cityCode, districtCode string, err error)
}

// GoshipDeps groups the goship provider's collaborators for the factory.
type GoshipDeps struct {
	Client     goship.Client
	PickupRepo PickupRepo
	Defaults   weight.Defaults
}

type GoshipProvider struct {
	client   goship.Client
	pickup   PickupRepo
	defaults weight.Defaults
}

func NewGoshipProvider(c goship.Client, p PickupRepo, d weight.Defaults) *GoshipProvider {
	return &GoshipProvider{client: c, pickup: p, defaults: d}
}

func (p *GoshipProvider) Quote(ctx context.Context, r CalcReq) ([]shippingdomain.ShippingOption, error) {
	if r.ToCityCode == nil || r.ToDistrict == nil {
		return nil, ErrDestinationIncomplete
	}
	fromCity, fromDist, err := p.pickup.PrimaryAddressCodes(ctx, r.BrandID)
	if err != nil {
		return nil, err
	}
	if fromCity == "" || fromDist == "" {
		return nil, ErrPickupIncomplete
	}

	wItems := make([]weight.Item, 0, len(r.Items))
	for _, it := range r.Items {
		wItems = append(wItems, weight.Item{
			Qty: it.Qty, WeightG: it.WeightG,
			LengthCM: it.LengthCM, WidthCM: it.WidthCM, HeightCM: it.HeightCM,
		})
	}
	parcel := weight.Aggregate(wItems, p.defaults)

	rates, err := p.client.Rates(ctx, goship.RateReq{
		From: goship.Address{CityCode: fromCity, DistrictCode: fromDist},
		To:   goship.Address{CityCode: *r.ToCityCode, DistrictCode: *r.ToDistrict},
		Parcel: goship.Parcel{
			WeightG: parcel.WeightG, LengthCM: parcel.LengthCM, WidthCM: parcel.WidthCM, HeightCM: parcel.HeightCM,
			CODVND: r.CODVND, AmountVND: r.AmountVND,
		},
	})
	if err != nil {
		return nil, err
	}
	opts := make([]shippingdomain.ShippingOption, 0, len(rates))
	for _, rt := range rates {
		opts = append(opts, shippingdomain.ShippingOption{
			Provider: "goship",
			Carrier: rt.Carrier, CarrierName: rt.CarrierName,
			Service: rt.Service, AmountVND: rt.FeeVND, ETA: rt.ETA,
		})
	}
	return opts, nil
}
