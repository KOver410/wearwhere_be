//go:build goship_real

package goship

import (
	"context"
	"os"
	"testing"
)

// TestRealGoship_CreateShipment_Gated confirms the /shipments contract against the
// live API. It obtains a fresh rate via Rates, then ONLY creates a real shipment
// when GOSHIP_ALLOW_REAL_CREATE=1 — because the configured token is a PRODUCTION
// token and a successful CreateShipment books a real delivery order. Without the
// opt-in it logs the would-be request and skips the POST.
func TestRealGoship_CreateShipment_Gated(t *testing.T) {
	c := realClient(t)
	from := Address{DistrictCode: os.Getenv("GOSHIP_TEST_FROM_DISTRICT"), CityCode: os.Getenv("GOSHIP_TEST_FROM_CITY")}
	to := Address{DistrictCode: os.Getenv("GOSHIP_TEST_TO_DISTRICT"), CityCode: os.Getenv("GOSHIP_TEST_TO_CITY")}
	if from.DistrictCode == "" || to.DistrictCode == "" {
		t.Skip("set GOSHIP_TEST_FROM_*/TO_* district+city codes to run the shipment-create check")
	}

	parcel := Parcel{WeightG: 1000, LengthCM: 20, WidthCM: 15, HeightCM: 10, AmountVND: 200000}
	rates, err := c.Rates(context.Background(), RateReq{From: from, To: to, Parcel: parcel})
	if err != nil {
		t.Fatalf("Rates: %v", err)
	}
	if len(rates) == 0 {
		t.Fatal("expected at least one rate to use for shipment creation")
	}
	rate := rates[0]
	t.Logf("would create shipment with rate id=%s carrier=%q fee=%d", rate.ID, rate.Carrier, rate.FeeVND)

	if os.Getenv("GOSHIP_ALLOW_REAL_CREATE") != "1" {
		t.Skip("GOSHIP_ALLOW_REAL_CREATE!=1 — skipping the real POST /shipments (token is production; would book a real order). " +
			"Set it to 1 ONLY against a sandbox account or when intentionally booking a test shipment.")
	}

	resp, err := c.CreateShipment(context.Background(), ShipmentReq{
		RateID: rate.ID,
		From:   ShipmentAddress{Name: "WearWhere Test", Phone: "0900000000", Street: "1 Test", DistrictCode: from.DistrictCode, CityCode: from.CityCode},
		To:     ShipmentAddress{Name: "Test Recipient", Phone: "0900000001", Street: "2 Test", DistrictCode: to.DistrictCode, CityCode: to.CityCode},
		Parcel: parcel,
		OrderRef: "goship-real-test",
	})
	if err != nil {
		t.Fatalf("CreateShipment: %v", err)
	}
	// Log the real response so the assumed /shipments field mapping in client_http.go
	// can be reconciled with the actual contract (spec §11).
	t.Logf("CreateShipment resp: tracking=%q gcode=%q label=%q fee=%d",
		resp.TrackingCode, resp.GoshipCode, resp.LabelURL, resp.FeeVND)
	if resp.TrackingCode == "" {
		t.Error("expected a non-empty tracking code from CreateShipment")
	}
}
