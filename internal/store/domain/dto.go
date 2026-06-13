package domain

import "github.com/google/uuid"

// StoreSummary is the list-item shape for nearby/search results.
type StoreSummary struct {
	ID             uuid.UUID `json:"id"`
	BrandName      string    `json:"brand_name"`
	BrandSlug      string    `json:"brand_slug"`
	LogoURL        *string   `json:"logo_url,omitempty"`
	Label          string    `json:"label"`
	AddressLine    string    `json:"address_line"`
	Ward           string    `json:"ward"`
	District       string    `json:"district"`
	City           string    `json:"city"`
	Latitude       float64   `json:"latitude"`
	Longitude      float64   `json:"longitude"`
	DistanceM      *int64    `json:"distance_m,omitempty"`
	DurationS      *int64    `json:"duration_s,omitempty"`
	DistanceApprox bool      `json:"distance_approx,omitempty"`
	Open           *bool     `json:"open,omitempty"`
}

// StoreDetail is the full single-store shape (UC25).
type StoreDetail struct {
	StoreSummary
	BannerURL *string         `json:"banner_url,omitempty"`
	Phone     *string         `json:"phone,omitempty"`
	Hours     []StoreHoursDTO `json:"hours"`
}

type StoreHoursDTO struct {
	Weekday   int    `json:"weekday"`
	OpenTime  string `json:"open_time"`
	CloseTime string `json:"close_time"`
}

// DirectionsResponse is the UC26 route payload.
type DirectionsResponse struct {
	DistanceM int64  `json:"distance_m"`
	DurationS int64  `json:"duration_s"`
	Polyline  string `json:"polyline"`
}

func openPtr(st *OpenStatus) *bool {
	if st == nil {
		return nil
	}
	return &st.Open
}

func ToStoreSummary(s *Store, open *OpenStatus) StoreSummary {
	return StoreSummary{
		ID: s.AddressID, BrandName: s.BrandName, BrandSlug: s.BrandSlug,
		LogoURL: s.LogoURL, Label: s.Label, AddressLine: s.AddressLine,
		Ward: s.Ward, District: s.District, City: s.City,
		Latitude: s.Latitude, Longitude: s.Longitude,
		DistanceM: s.DistanceM, DurationS: s.DurationS,
		DistanceApprox: s.DistanceApprox, Open: openPtr(open),
	}
}

func ToStoreDetail(s *Store, open *OpenStatus) StoreDetail {
	hrs := make([]StoreHoursDTO, 0, len(s.Hours))
	for _, h := range s.Hours {
		hrs = append(hrs, StoreHoursDTO{Weekday: h.Weekday, OpenTime: h.OpenTime, CloseTime: h.CloseTime})
	}
	return StoreDetail{
		StoreSummary: ToStoreSummary(s, open),
		BannerURL:    s.BannerURL, Phone: s.Phone, Hours: hrs,
	}
}
