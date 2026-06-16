package service

import (
	"context"

	productdomain "github.com/wearwhere/wearwhere_be/internal/product/domain"
	wdomain "github.com/wearwhere/wearwhere_be/internal/wardrobe/domain"
)

// Retriever fetches active in-stock products to suggest buying, filtered by
// style slugs and budget. Satisfied by CatalogRetriever (adapter over the
// product catalog service).
type Retriever interface {
	Retrieve(ctx context.Context, styleSlugs []string, budgetMin, budgetMax *int, limit int) ([]wdomain.OutfitCard, error)
}

// CatalogLister is the subset of product/service.CatalogService we use.
type CatalogLister interface {
	List(ctx context.Context, q *productdomain.ListProductsQuery) ([]*productdomain.CatalogItem, int, []string, error)
}

type CatalogRetriever struct{ catalog CatalogLister }

func NewCatalogRetriever(c CatalogLister) *CatalogRetriever { return &CatalogRetriever{catalog: c} }

func (r *CatalogRetriever) Retrieve(ctx context.Context, styleSlugs []string, budgetMin, budgetMax *int, limit int) ([]wdomain.OutfitCard, error) {
	q := &productdomain.ListProductsQuery{Page: 1, Limit: limit, Sort: "popular"}
	if len(styleSlugs) > 0 {
		// ListProductsQuery caps Style at 10 slugs.
		if len(styleSlugs) > 10 {
			styleSlugs = styleSlugs[:10]
		}
		q.Style = styleSlugs
	}
	if budgetMin != nil {
		v := float64(*budgetMin)
		q.PriceMin = &v
	}
	if budgetMax != nil {
		v := float64(*budgetMax)
		q.PriceMax = &v
	}
	items, _, _, err := r.catalog.List(ctx, q)
	if err != nil {
		return nil, err
	}
	out := make([]wdomain.OutfitCard, 0, len(items))
	for _, it := range items {
		// Spec §5.2: only suggest in-stock products to buy. The catalog list
		// returns active products regardless of stock (in_stock is informational),
		// so filter here.
		if !it.InStock {
			continue
		}
		out = append(out, catalogItemToCard(it))
	}
	return out, nil
}

func catalogItemToCard(it *productdomain.CatalogItem) wdomain.OutfitCard {
	return wdomain.OutfitCard{
		ID:           it.ID.String(),
		Slug:         it.Slug,
		Name:         it.Name,
		BrandSlug:    it.BrandSlug,
		BrandName:    it.BrandName,
		Currency:     it.Currency,
		MinPrice:     it.MinPrice,
		PrimaryImage: it.PrimaryImage,
	}
}
