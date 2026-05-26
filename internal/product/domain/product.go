package domain

import (
	"time"

	"github.com/google/uuid"
)

type ProductStatus string

const (
	ProductStatusDraft    ProductStatus = "draft"
	ProductStatusActive   ProductStatus = "active"
	ProductStatusArchived ProductStatus = "archived"
)

type Product struct {
	ID          uuid.UUID
	BrandID     uuid.UUID
	CategoryID  uuid.UUID
	Slug        string
	Name        string
	Description *string
	Status      ProductStatus
	Currency    string
	SoldCount   int
	ViewCount   int
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   *time.Time
}

type Variant struct {
	ID        uuid.UUID
	ProductID uuid.UUID
	SKU       string
	Size      string
	Color     string
	ColorHex  *string
	Price     float64
	StockQty  int
	IsActive  bool
	ImageID   *uuid.UUID
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
}

type Image struct {
	ID         uuid.UUID
	ProductID  uuid.UUID
	URL        string
	StorageKey string
	AltText    *string
	SortOrder  int
	IsPrimary  bool
	CreatedAt  time.Time
}

type Category struct {
	ID           uuid.UUID
	Slug         string
	Name         string
	DisplayOrder int
}

type StyleTag struct {
	ID   uuid.UUID
	Slug string
	Name string
}

// CatalogItem is a denormalized row for the public listing endpoint.
type CatalogItem struct {
	Product
	BrandSlug    string
	BrandName    string
	MinPrice     float64
	MaxPrice     float64
	InStock      bool
	PrimaryImage *string
}
