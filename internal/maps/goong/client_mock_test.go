package goong

import (
	"context"
	"testing"
)

func TestMockClient_DistanceMatrix_OnePerDest(t *testing.T) {
	m := NewMockClient()
	origin := LatLng{Lat: 10.776, Lng: 106.700}
	dests := []LatLng{{Lat: 10.78, Lng: 106.70}, {Lat: 10.80, Lng: 106.71}}
	got, err := m.DistanceMatrix(context.Background(), origin, dests)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(got) != len(dests) {
		t.Fatalf("got %d results, want %d", len(got), len(dests))
	}
	if got[1].DistanceM <= got[0].DistanceM {
		t.Errorf("expected dest1 farther than dest0: %d vs %d", got[1].DistanceM, got[0].DistanceM)
	}
}

func TestMockClient_Geocode_NonEmpty(t *testing.T) {
	m := NewMockClient()
	got, err := m.Geocode(context.Background(), "Quận 1, Hồ Chí Minh")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("expected at least one geocode result")
	}
}

func TestMockClient_Directions_Positive(t *testing.T) {
	m := NewMockClient()
	r, err := m.Directions(context.Background(), LatLng{10.77, 106.70}, LatLng{10.80, 106.71})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if r.DistanceM <= 0 || r.DurationS <= 0 || r.Polyline == "" {
		t.Errorf("expected positive route with polyline, got %+v", r)
	}
}
