package provider

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/wearwhere/wearwhere_be/internal/shipping/goship"
	"github.com/wearwhere/wearwhere_be/internal/shipping/weight"
)

type stubGoship struct{ rates []goship.Rate }

func (s stubGoship) Cities(context.Context) ([]goship.Location, error) { return nil, nil }
func (s stubGoship) Districts(context.Context, string) ([]goship.Location, error) {
	return nil, nil
}
func (s stubGoship) Wards(context.Context, string) ([]goship.Location, error) { return nil, nil }
func (s stubGoship) Rates(_ context.Context, _ goship.RateReq) ([]goship.Rate, error) {
	return s.rates, nil
}

type stubPickup struct {
	city, district string
	err            error
}

func (s stubPickup) PrimaryAddressCodes(_ context.Context, _ uuid.UUID) (city, district string, err error) {
	return s.city, s.district, s.err
}

func sp(s string) *string { return &s }

func TestGoshipProvider_Quote_MapsRates(t *testing.T) {
	cli := stubGoship{rates: []goship.Rate{
		{ID: "r1", Carrier: "ghnv3", CarrierName: "GHN", FeeVND: 25000, ETA: "2 ngày"},
		{ID: "r2", Carrier: "ghtk", CarrierName: "GHTK", FeeVND: 20000, ETA: "3 ngày"},
	}}
	d := weight.Defaults{WeightG: 500, LengthCM: 20, WidthCM: 15, HeightCM: 10}
	p := NewGoshipProvider(cli, stubPickup{city: "100000", district: "100100"}, d)

	opts, err := p.Quote(context.Background(), CalcReq{
		BrandID:    uuid.New(),
		ToCityCode: sp("200000"),
		ToDistrict: sp("200100"),
		Items:      []CalcItem{{Qty: 1}},
	})
	if err != nil {
		t.Fatalf("Quote: %v", err)
	}
	if len(opts) != 2 || opts[0].Carrier != "ghnv3" || opts[0].AmountVND != 25000 {
		t.Fatalf("unexpected options: %+v", opts)
	}
	if opts[1].Carrier != "ghtk" || opts[1].AmountVND != 20000 || opts[1].ETA != "3 ngày" {
		t.Fatalf("second option wrong: %+v", opts[1])
	}
}

func TestGoshipProvider_Quote_MissingDestCodes(t *testing.T) {
	p := NewGoshipProvider(stubGoship{}, stubPickup{city: "100000", district: "100100"}, weight.Defaults{})
	_, err := p.Quote(context.Background(), CalcReq{BrandID: uuid.New(), Items: []CalcItem{{Qty: 1}}})
	if err == nil {
		t.Fatal("expected error when destination codes are missing")
	}
}

func TestGoshipProvider_Quote_PickupIncomplete(t *testing.T) {
	cli := stubGoship{rates: []goship.Rate{{ID: "r1", Carrier: "ghnv3", CarrierName: "GHN", FeeVND: 25000}}}
	p := NewGoshipProvider(cli, stubPickup{city: "", district: ""}, weight.Defaults{})
	_, err := p.Quote(context.Background(), CalcReq{BrandID: uuid.New(), ToCityCode: sp("200000"), ToDistrict: sp("200100"), Items: []CalcItem{{Qty: 1}}})
	if !errors.Is(err, ErrPickupIncomplete) {
		t.Fatalf("want ErrPickupIncomplete, got %v", err)
	}
}
