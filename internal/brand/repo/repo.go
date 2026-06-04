// Package repo defines persistence interfaces for the brand module.
package repo

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/wearwhere/wearwhere_be/internal/brand/domain"
)

var ErrNotFound = errors.New("brand: not found")

type BrandRepo interface {
	FindByID(ctx context.Context, id uuid.UUID) (*domain.Brand, error)
	FindBySlug(ctx context.Context, slug string) (*domain.Brand, error)
	FindByOwnerUserID(ctx context.Context, userID uuid.UUID) (*domain.Brand, error)

	// Update applies non-nil fields. Returns ErrNotFound if brand id doesn't exist.
	// Returns a sentinel slug-conflict error (handler maps to SLUG_TAKEN).
	Update(ctx context.Context, id uuid.UUID, req *domain.UpdateBrandRequest) error

	// List paginated active brands; q is optional fuzzy match on name.
	List(ctx context.Context, q string, sort string, limit, offset int) ([]*domain.Brand, int, error)
}

type AddressRepo interface {
	List(ctx context.Context, brandID uuid.UUID, includePrivate bool) ([]*domain.BrandAddress, error)
	FindByID(ctx context.Context, id, brandID uuid.UUID) (*domain.BrandAddress, error)
	Create(ctx context.Context, brandID uuid.UUID, req *domain.CreateAddressRequest) (*domain.BrandAddress, error)
	Update(ctx context.Context, id, brandID uuid.UUID, req *domain.UpdateAddressRequest) (*domain.BrandAddress, error)
	SoftDelete(ctx context.Context, id, brandID uuid.UUID) error
	PrimaryAddress(ctx context.Context, brandID uuid.UUID) (*domain.BrandAddress, error)
}

// ErrSlugTaken is returned by BrandRepo.Update when the new slug collides.
var ErrSlugTaken = errors.New("brand: slug taken")
