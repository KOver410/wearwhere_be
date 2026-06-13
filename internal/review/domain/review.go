// Package domain holds product-review entities and DTOs.
package domain

import (
	"time"

	"github.com/google/uuid"
)

// Fit is the optional size/fit feedback on a review.
const (
	FitSmall = "small"
	FitTrue  = "true"
	FitLarge = "large"
)

// Review is a customer's review of a product. Every review is a verified
// purchase, so there is no separate "verified" flag.
type Review struct {
	ID        uuid.UUID
	ProductID uuid.UUID
	UserID    uuid.UUID
	Rating    int
	Body      string
	Fit       *string
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ReviewView is a Review joined with the reviewer's display name (for listing).
type ReviewView struct {
	Review
	ReviewerName string
}
