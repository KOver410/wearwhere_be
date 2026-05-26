package domain

import (
	"time"

	"github.com/google/uuid"
)

type UpdateBrandRequest struct {
	Name       *string `json:"name"        binding:"omitempty,min=2,max=120"`
	Slug       *string `json:"slug"        binding:"omitempty,slug,max=120"`
	Story      *string `json:"story"       binding:"omitempty,max=10000"`
	LogoURL    *string `json:"logo_url"    binding:"omitempty,url"`
	BannerURL  *string `json:"banner_url"  binding:"omitempty,url"`
	WebsiteURL *string `json:"website_url" binding:"omitempty,url"`
}

type CreateAddressRequest struct {
	Label       string   `json:"label"        binding:"required,max=80"`
	AddressLine string   `json:"address_line" binding:"required,max=255"`
	Ward        string   `json:"ward"         binding:"required,max=80"`
	District    string   `json:"district"     binding:"required,max=80"`
	City        string   `json:"city"         binding:"required,max=80"`
	Country     string   `json:"country"      binding:"omitempty,len=2"`
	PostalCode  *string  `json:"postal_code"  binding:"omitempty,max=20"`
	Phone       *string  `json:"phone"        binding:"omitempty,e164"`
	Latitude    *float64 `json:"latitude"     binding:"omitempty,latitude"`
	Longitude   *float64 `json:"longitude"    binding:"omitempty,longitude"`
	IsPrimary   bool     `json:"is_primary"`
	IsPublic    *bool    `json:"is_public"`
}

type UpdateAddressRequest struct {
	Label       *string  `json:"label"        binding:"omitempty,max=80"`
	AddressLine *string  `json:"address_line" binding:"omitempty,max=255"`
	Ward        *string  `json:"ward"         binding:"omitempty,max=80"`
	District    *string  `json:"district"     binding:"omitempty,max=80"`
	City        *string  `json:"city"         binding:"omitempty,max=80"`
	Country     *string  `json:"country"      binding:"omitempty,len=2"`
	PostalCode  *string  `json:"postal_code"  binding:"omitempty,max=20"`
	Phone       *string  `json:"phone"        binding:"omitempty,e164"`
	Latitude    *float64 `json:"latitude"     binding:"omitempty,latitude"`
	Longitude   *float64 `json:"longitude"    binding:"omitempty,longitude"`
	IsPrimary   *bool    `json:"is_primary"`
	IsPublic    *bool    `json:"is_public"`
}

type BrandResponse struct {
	ID         string  `json:"id"`
	Slug       string  `json:"slug"`
	Name       string  `json:"name"`
	Story      *string `json:"story,omitempty"`
	LogoURL    *string `json:"logo_url,omitempty"`
	BannerURL  *string `json:"banner_url,omitempty"`
	WebsiteURL *string `json:"website_url,omitempty"`
	Status     string  `json:"status"`
	CreatedAt  string  `json:"created_at"`
}

type AddressResponse struct {
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	AddressLine string   `json:"address_line"`
	Ward        string   `json:"ward"`
	District    string   `json:"district"`
	City        string   `json:"city"`
	Country     string   `json:"country"`
	PostalCode  *string  `json:"postal_code,omitempty"`
	Phone       *string  `json:"phone,omitempty"`
	Latitude    *float64 `json:"latitude,omitempty"`
	Longitude   *float64 `json:"longitude,omitempty"`
	IsPrimary   bool     `json:"is_primary"`
	IsPublic    bool     `json:"is_public"`
}

func ToBrandResponse(b *Brand) BrandResponse {
	return BrandResponse{
		ID:         b.ID.String(),
		Slug:       b.Slug,
		Name:       b.Name,
		Story:      b.Story,
		LogoURL:    b.LogoURL,
		BannerURL:  b.BannerURL,
		WebsiteURL: b.WebsiteURL,
		Status:     string(b.Status),
		CreatedAt:  b.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func ToAddressResponse(a *BrandAddress) AddressResponse {
	return AddressResponse{
		ID:          a.ID.String(),
		Label:       a.Label,
		AddressLine: a.AddressLine,
		Ward:        a.Ward,
		District:    a.District,
		City:        a.City,
		Country:     a.Country,
		PostalCode:  a.PostalCode,
		Phone:       a.Phone,
		Latitude:    a.Latitude,
		Longitude:   a.Longitude,
		IsPrimary:   a.IsPrimary,
		IsPublic:    a.IsPublic,
	}
}

// FindByOwnerResult bundles brand + ID for context middleware.
type FindByOwnerResult struct {
	BrandID uuid.UUID
	Status  BrandStatus
}
