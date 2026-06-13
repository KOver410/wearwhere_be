package goong

import (
	"context"
	"math"
)

// MockClient returns deterministic results derived from haversine distance,
// so tests are stable and ordering is realistic. No network calls.
type MockClient struct{}

func NewMockClient() *MockClient { return &MockClient{} }

// haversineM returns straight-line meters between two coordinates.
func haversineM(a, b LatLng) int64 {
	const earthM = 6371000.0
	la1 := a.Lat * math.Pi / 180
	la2 := b.Lat * math.Pi / 180
	dLa := (b.Lat - a.Lat) * math.Pi / 180
	dLo := (b.Lng - a.Lng) * math.Pi / 180
	h := math.Sin(dLa/2)*math.Sin(dLa/2) +
		math.Cos(la1)*math.Cos(la2)*math.Sin(dLo/2)*math.Sin(dLo/2)
	return int64(2 * earthM * math.Asin(math.Sqrt(h)))
}

func (m *MockClient) Geocode(_ context.Context, query string) ([]GeocodeResult, error) {
	return []GeocodeResult{{Lat: 10.7769, Lng: 106.7009, FormattedAddress: query}}, nil
}

func (m *MockClient) DistanceMatrix(_ context.Context, origin LatLng, dests []LatLng) ([]DistanceResult, error) {
	out := make([]DistanceResult, 0, len(dests))
	for _, d := range dests {
		straight := haversineM(origin, d)
		road := int64(float64(straight) * 1.3) // road factor
		out = append(out, DistanceResult{DistanceM: road, DurationS: road / 8})
	}
	return out, nil
}

func (m *MockClient) Directions(_ context.Context, origin, dest LatLng) (Route, error) {
	straight := haversineM(origin, dest)
	road := int64(float64(straight) * 1.3)
	return Route{DistanceM: road, DurationS: road / 8, Polyline: "mock_polyline"}, nil
}
