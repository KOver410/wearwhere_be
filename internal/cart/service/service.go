package service

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/wearwhere/wearwhere_be/internal/cart/domain"
	"github.com/wearwhere/wearwhere_be/internal/cart/repo"
	productrepo "github.com/wearwhere/wearwhere_be/internal/product/repo"
)

type Service struct {
	cart    repo.CartRepo
	variant productrepo.VariantRepo
}

func New(c repo.CartRepo, v productrepo.VariantRepo) *Service {
	return &Service{cart: c, variant: v}
}

func (s *Service) Add(ctx context.Context, userID uuid.UUID, variantID uuid.UUID, qty int) (*domain.CartItem, error) {
	if qty < 1 || qty > 10 {
		return nil, domain.ErrQtyExceedsMax
	}
	v, _, err := s.variant.FindForPurchase(ctx, variantID)
	if err != nil {
		if errors.Is(err, productrepo.ErrNotFound) {
			return nil, domain.ErrVariantUnavailable
		}
		return nil, err
	}

	// Cumulative qty check against the 10-max and live stock.
	existing, findErr := s.cart.FindByVariant(ctx, userID, variantID)
	cumulative := qty
	if findErr == nil && existing != nil {
		cumulative += existing.Qty
		if cumulative > 10 {
			return nil, domain.ErrQtyExceedsMax
		}
	}
	if v.StockQty < cumulative {
		return nil, domain.ErrOutOfStock
	}

	return s.cart.UpsertAdd(ctx, userID, variantID, qty, v.Price)
}

func (s *Service) UpdateQty(ctx context.Context, id, userID uuid.UUID, qty int) (*domain.CartItem, error) {
	if qty < 1 || qty > 10 {
		return nil, domain.ErrQtyExceedsMax
	}
	item, err := s.cart.FindByID(ctx, id, userID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, domain.ErrCartItemNotFound
		}
		return nil, err
	}
	v, _, err := s.variant.FindForPurchase(ctx, item.VariantID)
	if err != nil {
		return nil, domain.ErrVariantUnavailable
	}
	if v.StockQty < qty {
		return nil, domain.ErrOutOfStock
	}
	return s.cart.UpdateQty(ctx, id, userID, qty, v.Price)
}

func (s *Service) Remove(ctx context.Context, id, userID uuid.UUID) error {
	if err := s.cart.Delete(ctx, id, userID); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return domain.ErrCartItemNotFound
		}
		return err
	}
	return nil
}

func (s *Service) Clear(ctx context.Context, userID uuid.UUID) error {
	return s.cart.Clear(ctx, userID)
}

func (s *Service) List(ctx context.Context, userID uuid.UUID) ([]*domain.CartItemView, error) {
	return s.cart.ListView(ctx, userID)
}
