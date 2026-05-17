package domain

import (
	"time"

	"github.com/google/uuid"
)

type BrandStatus string

const (
	BrandStatusPending   BrandStatus = "pending"
	BrandStatusActive    BrandStatus = "active"
	BrandStatusSuspended BrandStatus = "suspended"
)

type Brand struct {
	ID          uuid.UUID
	Slug        string
	Name        string
	OwnerUserID uuid.UUID
	Story       *string
	LogoURL     *string
	BannerURL   *string
	WebsiteURL  *string
	Status      BrandStatus
	VerifiedAt  *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   *time.Time
}

type BrandAddress struct {
	ID          uuid.UUID
	BrandID     uuid.UUID
	Label       string
	AddressLine string
	Ward        string
	District    string
	City        string
	Country     string
	PostalCode  *string
	Phone       *string
	Latitude    *float64
	Longitude   *float64
	IsPrimary   bool
	IsPublic    bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   *time.Time
}
