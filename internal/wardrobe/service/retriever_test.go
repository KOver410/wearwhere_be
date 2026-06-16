package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	productdomain "github.com/wearwhere/wearwhere_be/internal/product/domain"
	"github.com/wearwhere/wearwhere_be/internal/wardrobe/service"
)

type fakeCatalog struct{ items []*productdomain.CatalogItem }

func (f *fakeCatalog) List(_ context.Context, _ *productdomain.ListProductsQuery) ([]*productdomain.CatalogItem, int, []string, error) {
	return f.items, len(f.items), nil, nil
}

func TestCatalogRetriever_FiltersOutOfStock(t *testing.T) {
	inStock := &productdomain.CatalogItem{InStock: true}
	inStock.ID = uuid.New()
	inStock.Name = "In"
	oos := &productdomain.CatalogItem{InStock: false}
	oos.ID = uuid.New()
	oos.Name = "Out"

	r := service.NewCatalogRetriever(&fakeCatalog{items: []*productdomain.CatalogItem{inStock, oos}})
	cards, err := r.Retrieve(context.Background(), nil, nil, nil, 10)
	require.NoError(t, err)
	require.Len(t, cards, 1, "out-of-stock product must be filtered from to_buy")
	require.Equal(t, "In", cards[0].Name)
}
