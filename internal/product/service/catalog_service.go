package service

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/wearwhere/wearwhere_be/internal/product/domain"
	"github.com/wearwhere/wearwhere_be/internal/product/repo"
)

type CatalogService struct {
	catalog  repo.CatalogRepo
	products repo.ProductRepo
}

func NewCatalog(c repo.CatalogRepo, p repo.ProductRepo) *CatalogService {
	return &CatalogService{catalog: c, products: p}
}

// List returns (items, total, suggestions, err). Suggestions only when q is
// non-empty AND there were zero results.
func (s *CatalogService) List(ctx context.Context, q *domain.ListProductsQuery) ([]*domain.CatalogItem, int, []string, error) {
	items, total, err := s.catalog.List(ctx, q)
	if err != nil {
		return nil, 0, nil, err
	}
	var suggestions []string
	if total == 0 && q.Q != "" {
		suggestions, _ = s.catalog.Suggestions(ctx, q.Q, 3)
	}
	return items, total, suggestions, nil
}

func (s *CatalogService) Detail(ctx context.Context, brandSlug, productSlug string) (
	*domain.Product, *domain.Category, []*domain.Variant, []*domain.Image, []*domain.StyleTag, error,
) {
	p, cat, vs, imgs, tags, err := s.catalog.Detail(ctx, brandSlug, productSlug)
	if errors.Is(err, repo.ErrNotFound) {
		return nil, nil, nil, nil, nil, domain.ErrProductNotFound
	}
	if err == nil {
		// fire-and-forget view increment
		go func(id uuid.UUID) {
			_ = s.products.IncrementViewCount(context.Background(), id)
		}(p.ID)
	}
	return p, cat, vs, imgs, tags, err
}

func (s *CatalogService) DetailByID(ctx context.Context, id uuid.UUID) (
	*domain.Product, *domain.Category, []*domain.Variant, []*domain.Image, []*domain.StyleTag, error,
) {
	p, cat, vs, imgs, tags, err := s.catalog.DetailByID(ctx, id)
	if errors.Is(err, repo.ErrNotFound) {
		return nil, nil, nil, nil, nil, domain.ErrProductNotFound
	}
	if err == nil {
		go func(id uuid.UUID) {
			_ = s.products.IncrementViewCount(context.Background(), id)
		}(p.ID)
	}
	return p, cat, vs, imgs, tags, err
}
