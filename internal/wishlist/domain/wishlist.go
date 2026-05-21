package domain

import (
	"time"

	"github.com/google/uuid"
)

type WishlistItem struct {
	UserID    uuid.UUID
	ProductID uuid.UUID
	AddedAt   time.Time
}

// WishlistItemView is the denormalized row returned by GET /me/wishlist.
type WishlistItemView struct {
	ProductID       uuid.UUID
	ProductSlug     string
	ProductName     string
	PrimaryImageURL *string
	MinPrice        *float64
	BrandID         uuid.UUID
	BrandSlug       string
	BrandName       string
	AddedAt         time.Time
}
