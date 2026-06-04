//go:build goship_real

package goship

import (
	"context"
	"os"
	"testing"
)

func realClient(t *testing.T) *HTTPClient {
	tok := os.Getenv("GOSHIP_TOKEN")
	if tok == "" {
		t.Skip("GOSHIP_TOKEN not set; skipping real Goship test")
	}
	base := os.Getenv("GOSHIP_BASE_URL")
	if base == "" {
		base = "https://sandbox.goship.io/api/v2"
	}
	return NewHTTPClient(tok, "", base)
}

func TestRealGoship_Cities(t *testing.T) {
	c := realClient(t)
	cities, err := c.Cities(context.Background())
	if err != nil {
		t.Fatalf("Cities: %v", err)
	}
	if len(cities) == 0 {
		t.Fatal("expected at least one city")
	}
	t.Logf("got %d cities; first=%+v", len(cities), cities[0])
}

func TestRealGoship_Rates(t *testing.T) {
	c := realClient(t)
	from := Address{DistrictCode: os.Getenv("GOSHIP_TEST_FROM_DISTRICT"), CityCode: os.Getenv("GOSHIP_TEST_FROM_CITY")}
	to := Address{DistrictCode: os.Getenv("GOSHIP_TEST_TO_DISTRICT"), CityCode: os.Getenv("GOSHIP_TEST_TO_CITY")}
	if from.DistrictCode == "" || to.DistrictCode == "" {
		t.Skip("set GOSHIP_TEST_FROM_*/TO_* district+city codes to run rates test")
	}
	rates, err := c.Rates(context.Background(), RateReq{From: from, To: to, Parcel: Parcel{WeightG: 1000, LengthCM: 20, WidthCM: 15, HeightCM: 10, AmountVND: 500000}})
	if err != nil {
		t.Fatalf("Rates: %v", err)
	}
	if len(rates) == 0 {
		t.Fatal("expected at least one carrier rate")
	}
	for _, r := range rates {
		t.Logf("rate id=%s carrier=%q name=%q fee=%d eta=%q", r.ID, r.Carrier, r.CarrierName, r.FeeVND, r.ETA)
	}
}
