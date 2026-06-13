// Package repo defines persistence for product reviews.
package repo

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/wearwhere/wearwhere_be/internal/review/domain"
)

var (
	ErrNotFound  = errors.New("review: not found")
	ErrDuplicate = errors.New("review: already exists for this user+product")
)

// ListFilter is the normalized query for ListByProduct.
type ListFilter struct {
	Rating int    // 0 = no filter
	Fit    string // "" = no filter
	Sort   string // newest | rating_high | rating_low
	Limit  int
	Offset int
}

// Aggregate is the denormalized rating summary stored on products.
type Aggregate struct {
	AvgRating   float64
	ReviewCount int
}

type Repo interface {
	ProductExists(ctx context.Context, productID uuid.UUID) (bool, error)
	HasDeliveredPurchase(ctx context.Context, userID, productID uuid.UUID) (bool, error)
	Create(ctx context.Context, r *domain.Review) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Review, error)
	Update(ctx context.Context, id uuid.UUID, rating int, body string, fit *string) error
	SoftDelete(ctx context.Context, id uuid.UUID) error
	ListByProduct(ctx context.Context, productID uuid.UUID, f ListFilter) ([]*domain.ReviewView, int, error)
	Aggregate(ctx context.Context, productID uuid.UUID) (Aggregate, error)
}
