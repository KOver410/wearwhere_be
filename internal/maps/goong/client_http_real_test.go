package goong

import (
	"context"
	"os"
	"testing"
)

// Live test against the real Goong API. Skipped unless GOONG_API_KEY is set,
// mirroring goship/client_http_real_test.go.
func TestHTTPClient_DistanceMatrix_Real(t *testing.T) {
	key := os.Getenv("GOONG_API_KEY")
	if key == "" {
		t.Skip("GOONG_API_KEY not set; skipping live Goong test")
	}
	c := NewHTTPClient(key, "https://rsapi.goong.io")
	origin := LatLng{Lat: 10.7769, Lng: 106.7009}
	dests := []LatLng{{Lat: 10.7800, Lng: 106.7000}}
	got, err := c.DistanceMatrix(context.Background(), origin, dests)
	if err != nil {
		t.Fatalf("DistanceMatrix: %v", err)
	}
	if len(got) != 1 || got[0].DistanceM <= 0 {
		t.Fatalf("unexpected result: %+v", got)
	}
}
