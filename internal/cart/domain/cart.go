package domain

import (
	"time"

	"github.com/google/uuid"
)

type CartItem struct {
	ID               uuid.UUID
	UserID           uuid.UUID
	VariantID        uuid.UUID
	Qty              int
	PriceSnapshot    float64
	CurrencySnapshot string
	AddedAt          time.Time
	UpdatedAt        time.Time
}

// CartItemView is the denormalized row returned by GET /me/cart.
type CartItemView struct {
	ID               uuid.UUID
	Qty              int
	PriceSnapshot    float64
	CurrentPrice     float64
	CurrencySnapshot string
	AddedAt          time.Time

	VariantID uuid.UUID
	SKU       string
	Size      string
	Color     string
	ColorHex  *string
	StockQty  int

	ProductID       uuid.UUID
	ProductSlug     string
	ProductName     string
	PrimaryImageURL *string

	BrandID   uuid.UUID
	BrandSlug string
	BrandName string

	Unavailable       bool
	UnavailableReason *string // "variant_inactive" | "variant_deleted" | "product_unavailable"
}
