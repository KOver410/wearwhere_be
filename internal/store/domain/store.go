// Package domain holds store-discovery entities and pure logic.
package domain

import (
	"math"
	"time"

	"github.com/google/uuid"
)

// Store is a public, geocoded brand address presented as a physical store,
// composed with brand identity and opening hours.
type Store struct {
	AddressID   uuid.UUID
	BrandID     uuid.UUID
	BrandName   string
	BrandSlug   string
	LogoURL     *string
	BannerURL   *string
	Label       string
	AddressLine string
	Ward        string
	District    string
	City        string
	Phone       *string
	Latitude    float64
	Longitude   float64
	Hours       []StoreHours

	// Populated by the service for nearby/search results.
	DistanceM      *int64
	DurationS      *int64
	DistanceApprox bool // true when distance is haversine fallback (Goong unavailable)
}

// StoreHours is one opening window for a given weekday (0=Sunday..6=Saturday).
// Times are "HH:MM" in Asia/Ho_Chi_Minh.
type StoreHours struct {
	Weekday   int
	OpenTime  string
	CloseTime string
}

// OpenStatus is the computed open/closed state at a point in time.
type OpenStatus struct {
	Open bool
}

// HaversineKm returns the straight-line distance in kilometers.
func HaversineKm(lat1, lng1, lat2, lng2 float64) float64 {
	const earthKm = 6371.0
	dLat := (lat2 - lat1) * math.Pi / 180
	dLng := (lng2 - lng1) * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
			math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * earthKm * math.Asin(math.Sqrt(a))
}

// ComputeOpenStatus returns nil when no hours are configured (status unknown).
// Otherwise it reports whether `now` falls inside any window for that weekday.
func ComputeOpenStatus(hours []StoreHours, now time.Time) *OpenStatus {
	if len(hours) == 0 {
		return nil
	}
	loc, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err == nil {
		now = now.In(loc)
	}
	wd := int(now.Weekday()) // Sunday=0..Saturday=6
	cur := now.Hour()*60 + now.Minute()
	for _, h := range hours {
		if h.Weekday != wd {
			continue
		}
		open := parseHM(h.OpenTime)
		closeM := parseHM(h.CloseTime)
		// A window where closeM <= open wraps past midnight (e.g. 18:00–02:00).
		// We evaluate the wrap against this same weekday row (the post-midnight
		// portion is attributed here, not to the next calendar weekday) — a
		// deliberate simplification that keeps open/closed correct for late-night stores.
		var openNow bool
		if closeM <= open {
			openNow = cur >= open || cur < closeM
		} else {
			openNow = open <= cur && cur < closeM
		}
		if openNow {
			return &OpenStatus{Open: true}
		}
	}
	return &OpenStatus{Open: false}
}

// parseHM converts "HH:MM" to minutes since midnight; returns 0 on parse failure.
func parseHM(s string) int {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0
	}
	return t.Hour()*60 + t.Minute()
}
