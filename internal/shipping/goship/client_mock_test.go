package goship

import (
	"context"
	"testing"
)

func TestMock_Rates_DeterministicByWeight(t *testing.T) {
	m := NewMockClient()
	got, err := m.Rates(context.Background(), RateReq{
		From:   Address{DistrictCode: "100100", CityCode: "100000"},
		To:     Address{DistrictCode: "200100", CityCode: "200000"},
		Parcel: Parcel{WeightG: 1500},
	})
	if err != nil {
		t.Fatalf("Rates: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 carriers, got %d", len(got))
	}
	// "Giao Hàng Nhanh (v3)" base 15000 + 5000/kg * ceil(1.5kg=2) = 25000.
	// Mock mirrors prod: Carrier == CarrierName (display name, no short code).
	for _, r := range got {
		if r.Carrier == "Giao Hàng Nhanh (v3)" && r.FeeVND != 25000 {
			t.Errorf("GHN fee = %d, want 25000", r.FeeVND)
		}
		if r.Carrier != r.CarrierName {
			t.Errorf("Carrier should equal CarrierName (no short code): %+v", r)
		}
		if r.ID == "" || r.CarrierName == "" {
			t.Errorf("rate missing id/name: %+v", r)
		}
	}
}

func TestMock_Cities_NonEmpty(t *testing.T) {
	m := NewMockClient()
	c, err := m.Cities(context.Background())
	if err != nil || len(c) == 0 {
		t.Fatalf("Cities empty/err: %v", err)
	}
}
