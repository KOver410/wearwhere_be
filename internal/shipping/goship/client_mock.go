package goship

import (
	"context"
	"fmt"
	"math"
)

type MockClient struct{}

func NewMockClient() *MockClient { return &MockClient{} }

func (m *MockClient) Cities(_ context.Context) ([]Location, error) {
	return []Location{{Code: "100000", Name: "Hồ Chí Minh"}, {Code: "200000", Name: "Hà Nội"}}, nil
}

func (m *MockClient) Districts(_ context.Context, cityCode string) ([]Location, error) {
	return []Location{{Code: cityCode + "100", Name: "Quận 1"}, {Code: cityCode + "200", Name: "Quận 2"}}, nil
}

func (m *MockClient) Wards(_ context.Context, districtCode string) ([]Location, error) {
	return []Location{{Code: districtCode + "01", Name: "Phường 1"}}, nil
}

func (m *MockClient) Rates(_ context.Context, r RateReq) ([]Rate, error) {
	kg := int(math.Ceil(float64(r.Parcel.WeightG) / 1000.0))
	if kg < 1 {
		kg = 1
	}
	// Mirror the real Goship contract: the /rates response exposes NO short carrier
	// code — only a display name (confirmed against api.goship.io). Our HTTP client
	// sets Carrier = CarrierName in that case, so the mock does the same here
	// (Carrier == CarrierName == display name).
	carriers := []struct {
		name        string
		base, perKg int64
	}{
		{"Giao Hàng Nhanh (v3)", 15000, 5000},
		{"Vietnam Post", 12000, 4000},
		{"Viettel Post", 18000, 6000},
	}
	out := make([]Rate, 0, len(carriers))
	for i, c := range carriers {
		out = append(out, Rate{
			ID:          fmt.Sprintf("mock-rate-%d", i),
			Carrier:     c.name,
			CarrierName: c.name,
			Service:     "standard",
			FeeVND:      c.base + c.perKg*int64(kg),
			ETA:         "2-4 ngày",
		})
	}
	return out, nil
}
