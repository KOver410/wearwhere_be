package service

import (
	"context"

	"github.com/google/uuid"

	productrepo "github.com/wearwhere/wearwhere_be/internal/product/repo"
	"github.com/wearwhere/wearwhere_be/internal/wishlist/domain"
	"github.com/wearwhere/wearwhere_be/internal/wishlist/repo"
)

type Service struct {
	wishlist    repo.WishlistRepo
	productRepo productrepo.ProductRepo
}

func New(w repo.WishlistRepo, p productrepo.ProductRepo) *Service {
	return &Service{wishlist: w, productRepo: p}
}

// Add gates on product existence + active + not soft-deleted. All three
// failure modes collapse to ErrProductNotFound — the caller (frontend) does
// not need to disambiguate.
func (s *Service) Add(ctx context.Context, userID, productID uuid.UUID) error {
	p, err := s.productRepo.FindByID(ctx, productID)
	if err != nil || p == nil {
		return domain.ErrProductNotFound
	}
	if string(p.Status) != "active" || p.DeletedAt != nil {
		return domain.ErrProductNotFound
	}
	return s.wishlist.Add(ctx, userID, productID)
}

// Remove is idempotent — never errors on missing row.
func (s *Service) Remove(ctx context.Context, userID, productID uuid.UUID) error {
	return s.wishlist.Remove(ctx, userID, productID)
}

func (s *Service) List(ctx context.Context, userID uuid.UUID, page, limit int) ([]*domain.WishlistItemView, int, error) {
	return s.wishlist.List(ctx, userID, limit, (page-1)*limit)
}

// Contains backfills false for IDs the user does not have wishlisted so the
// HTTP response includes every requested ID as a key.
func (s *Service) Contains(ctx context.Context, userID uuid.UUID, productIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	hits, err := s.wishlist.Contains(ctx, userID, productIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]bool, len(productIDs))
	for _, id := range productIDs {
		out[id] = hits[id]
	}
	return out, nil
}
