package domain

import (
	"time"

	"github.com/google/uuid"
)

// RecProductCard is the public product card in the feed. Mirrors the catalog
// summary fields used elsewhere (id/slug/name/brand/price/image).
type RecProductCard struct {
	ID           string  `json:"id"`
	Slug         string  `json:"slug"`
	Name         string  `json:"name"`
	BrandSlug    string  `json:"brand_slug"`
	BrandName    string  `json:"brand_name"`
	Currency     string  `json:"currency"`
	MinPrice     float64 `json:"min_price"`
	PrimaryImage *string `json:"primary_image,omitempty"`
}

// RecommendationsResponse is the GET /me/recommendations body.
type RecommendationsResponse struct {
	Items            []RecProductCard `json:"items"`
	Source           string           `json:"source"` // "personalized" | "trending"
	OnboardingPrompt bool             `json:"onboarding_prompt"`
}

// Candidate is an internal scoring row: a catalog product plus the attributes
// the scorer needs. Not serialized to clients directly.
type Candidate struct {
	ProductID    uuid.UUID
	BrandID      uuid.UUID
	CategoryID   uuid.UUID
	Slug         string
	Name         string
	BrandSlug    string
	BrandName    string
	Currency     string
	MinPrice     float64
	PrimaryImage *string
	SoldCount    int
	CreatedAt    time.Time
	StyleTagIDs  []uuid.UUID
}

// ToCard projects a Candidate to its public card.
func (c Candidate) ToCard() RecProductCard {
	return RecProductCard{
		ID:           c.ProductID.String(),
		Slug:         c.Slug,
		Name:         c.Name,
		BrandSlug:    c.BrandSlug,
		BrandName:    c.BrandName,
		Currency:     c.Currency,
		MinPrice:     c.MinPrice,
		PrimaryImage: c.PrimaryImage,
	}
}

// UserSignals is the assembled per-user signal set the scorer reads.
// Maps are used for O(1) membership. HasProfile/HasHistory drive warm-vs-cold.
type UserSignals struct {
	StyleTagIDs         map[uuid.UUID]bool
	BudgetMin           *int
	BudgetMax           *int
	FollowedBrandIDs    map[uuid.UUID]bool
	PurchasedProductIDs map[uuid.UUID]bool
	AffinityCategoryIDs map[uuid.UUID]bool
}

// HasProfile is true when the user set any style tags or a budget.
func (s UserSignals) HasProfile() bool {
	return len(s.StyleTagIDs) > 0 || s.BudgetMin != nil || s.BudgetMax != nil
}

// HasHistory is true when the user has any behavioral signal.
func (s UserSignals) HasHistory() bool {
	return len(s.FollowedBrandIDs) > 0 || len(s.PurchasedProductIDs) > 0 || len(s.AffinityCategoryIDs) > 0
}
