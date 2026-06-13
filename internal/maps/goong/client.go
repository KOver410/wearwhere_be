// Package goong is a server-side adapter for the Goong Maps API
// (geocoding, distance matrix, directions). Mirrors internal/shipping/goship.
package goong

import (
	"context"
	"errors"
)

var (
	ErrGeocode    = errors.New("goong: failed to geocode")
	ErrDistance   = errors.New("goong: failed to fetch distance matrix")
	ErrDirections = errors.New("goong: failed to fetch directions")
)

// LatLng is a WGS84 coordinate.
type LatLng struct {
	Lat float64
	Lng float64
}

// GeocodeResult is one candidate for a geocoded query.
type GeocodeResult struct {
	Lat              float64
	Lng              float64
	FormattedAddress string
}

// DistanceResult is the road distance/duration from one origin to one destination.
type DistanceResult struct {
	DistanceM int64 // meters
	DurationS int64 // seconds
}

// Route is a single computed route for directions.
type Route struct {
	DistanceM int64
	DurationS int64
	Polyline  string // encoded polyline for the client to render
}

type Client interface {
	Geocode(ctx context.Context, query string) ([]GeocodeResult, error)
	// DistanceMatrix returns one DistanceResult per destination, in the same
	// order as dests. Used to enrich nearby candidates with real road distance.
	DistanceMatrix(ctx context.Context, origin LatLng, dests []LatLng) ([]DistanceResult, error)
	Directions(ctx context.Context, origin, dest LatLng) (Route, error)
}
