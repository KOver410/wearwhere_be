//go:build integration

package repo

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/product/domain"
	"github.com/wearwhere/wearwhere_be/internal/testfixtures"
)

// mkActiveProduct seeds an active product with one variant and overrides its name.
func mkActiveProduct(t *testing.T, db testfixtures.DBTX, brandID, categoryID uuid.UUID, name string, price float64, size, color string) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	p := testfixtures.SeedProduct(t, db, brandID, categoryID, "active")
	_, err := db.Exec(ctx, `UPDATE products SET name=$1 WHERE id=$2`, name, p.ID)
	require.NoError(t, err)
	testfixtures.SeedVariant(t, db, p.ID, size, color, price, 10)
	return p.ID
}

func TestCatalog_List_NoFilters(t *testing.T) {
	tx := testfixtures.BeginTx(t, testPool)
	sb := testfixtures.SeedBrand(t, tx, uuid.Nil)
	sc := testfixtures.SeedCategory(t, tx)
	mkActiveProduct(t, tx, sb.ID, sc.ID, "Áo Thun Trắng", 250000, "M", "White")
	mkActiveProduct(t, tx, sb.ID, sc.ID, "Quần Jeans Xanh", 500000, "L", "Blue")

	repo := NewCatalogPG(tx)
	items, total, err := repo.List(context.Background(), &domain.ListProductsQuery{
		Page: 1, Limit: 10, Sort: "newest",
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, total, 2)
	require.GreaterOrEqual(t, len(items), 2)
}

func TestCatalog_List_SearchByName(t *testing.T) {
	tx := testfixtures.BeginTx(t, testPool)
	sb := testfixtures.SeedBrand(t, tx, uuid.Nil)
	sc := testfixtures.SeedCategory(t, tx)
	mkActiveProduct(t, tx, sb.ID, sc.ID, "Áo Thun Trắng", 250000, "M", "White")
	mkActiveProduct(t, tx, sb.ID, sc.ID, "Quần Jeans", 500000, "L", "Blue")

	repo := NewCatalogPG(tx)
	// Search with accents stripped — should match "Áo Thun Trắng"
	items, _, err := repo.List(context.Background(), &domain.ListProductsQuery{
		Q: "ao thun", Page: 1, Limit: 10, Sort: "relevance",
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(items), 1)
	found := false
	for _, i := range items {
		if i.Name == "Áo Thun Trắng" {
			found = true
		}
	}
	require.True(t, found, "expected áo thun trắng in results")
}

func TestCatalog_List_FilterByPriceRange(t *testing.T) {
	tx := testfixtures.BeginTx(t, testPool)
	sb := testfixtures.SeedBrand(t, tx, uuid.Nil)
	sc := testfixtures.SeedCategory(t, tx)
	mkActiveProduct(t, tx, sb.ID, sc.ID, "Cheap", 100000, "M", "White")
	mkActiveProduct(t, tx, sb.ID, sc.ID, "Pricey", 1000000, "M", "Black")

	repo := NewCatalogPG(tx)
	min := 50000.0
	max := 200000.0
	items, _, err := repo.List(context.Background(), &domain.ListProductsQuery{
		PriceMin: &min, PriceMax: &max, Page: 1, Limit: 10, Sort: "newest",
		Brand: sb.Slug,
	})
	require.NoError(t, err)
	for _, i := range items {
		require.LessOrEqual(t, i.MinPrice, 200000.0)
		require.GreaterOrEqual(t, i.MinPrice, 50000.0)
	}
}

func TestCatalog_List_FilterByBrandSlug(t *testing.T) {
	tx := testfixtures.BeginTx(t, testPool)
	sbA := testfixtures.SeedBrand(t, tx, uuid.Nil)
	sbB := testfixtures.SeedBrand(t, tx, uuid.Nil)
	sc := testfixtures.SeedCategory(t, tx)
	mkActiveProduct(t, tx, sbA.ID, sc.ID, "From A", 100, "M", "X")
	mkActiveProduct(t, tx, sbB.ID, sc.ID, "From B", 100, "M", "X")

	repo := NewCatalogPG(tx)
	items, _, err := repo.List(context.Background(), &domain.ListProductsQuery{
		Brand: sbA.Slug, Page: 1, Limit: 10, Sort: "newest",
	})
	require.NoError(t, err)
	for _, i := range items {
		require.Equal(t, sbA.Slug, i.BrandSlug)
	}
}

func TestCatalog_List_SortPriceAsc(t *testing.T) {
	tx := testfixtures.BeginTx(t, testPool)
	sb := testfixtures.SeedBrand(t, tx, uuid.Nil)
	sc := testfixtures.SeedCategory(t, tx)
	mkActiveProduct(t, tx, sb.ID, sc.ID, "P1", 300, "M", "X")
	mkActiveProduct(t, tx, sb.ID, sc.ID, "P2", 100, "M", "Y")
	mkActiveProduct(t, tx, sb.ID, sc.ID, "P3", 200, "M", "Z")

	repo := NewCatalogPG(tx)
	items, _, err := repo.List(context.Background(), &domain.ListProductsQuery{
		Page: 1, Limit: 10, Sort: "price_asc", Brand: sb.Slug,
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(items), 3)
	// Verify non-decreasing min_price for our products
	var prev float64
	for _, i := range items {
		require.GreaterOrEqual(t, i.MinPrice, prev)
		prev = i.MinPrice
	}
}

func TestCatalog_List_IncludesOnlyActive(t *testing.T) {
	tx := testfixtures.BeginTx(t, testPool)
	sb := testfixtures.SeedBrand(t, tx, uuid.Nil)
	sc := testfixtures.SeedCategory(t, tx)
	pDraft := testfixtures.SeedProduct(t, tx, sb.ID, sc.ID, "draft")
	testfixtures.SeedVariant(t, tx, pDraft.ID, "M", "X", 100, 10)
	pActive := testfixtures.SeedProduct(t, tx, sb.ID, sc.ID, "active")
	testfixtures.SeedVariant(t, tx, pActive.ID, "M", "Y", 100, 10)

	repo := NewCatalogPG(tx)
	items, _, err := repo.List(context.Background(), &domain.ListProductsQuery{
		Brand: sb.Slug, Page: 1, Limit: 100, Sort: "newest",
	})
	require.NoError(t, err)
	for _, i := range items {
		require.NotEqual(t, pDraft.ID, i.ID)
	}
}

func TestCatalog_Detail(t *testing.T) {
	tx := testfixtures.BeginTx(t, testPool)
	sb := testfixtures.SeedBrand(t, tx, uuid.Nil)
	sc := testfixtures.SeedCategory(t, tx)
	p := testfixtures.SeedProduct(t, tx, sb.ID, sc.ID, "active")
	testfixtures.SeedVariant(t, tx, p.ID, "S", "Red", 100, 5)
	testfixtures.SeedVariant(t, tx, p.ID, "M", "Red", 100, 3)

	repo := NewCatalogPG(tx)
	prod, cat, variants, _, _, err := repo.Detail(context.Background(), sb.Slug, p.Slug)
	require.NoError(t, err)
	require.Equal(t, p.ID, prod.ID)
	require.Equal(t, sc.ID, cat.ID)
	require.Len(t, variants, 2)
}

func TestCatalog_DetailByID(t *testing.T) {
	tx := testfixtures.BeginTx(t, testPool)
	sb := testfixtures.SeedBrand(t, tx, uuid.Nil)
	sc := testfixtures.SeedCategory(t, tx)
	p := testfixtures.SeedProduct(t, tx, sb.ID, sc.ID, "active")
	testfixtures.SeedVariant(t, tx, p.ID, "M", "Blue", 200, 5)

	repo := NewCatalogPG(tx)
	prod, cat, variants, _, _, err := repo.DetailByID(context.Background(), p.ID)
	require.NoError(t, err)
	require.Equal(t, p.ID, prod.ID)
	require.Equal(t, sc.ID, cat.ID)
	require.Len(t, variants, 1)
}

func TestCatalog_Suggestions(t *testing.T) {
	tx := testfixtures.BeginTx(t, testPool)
	sb := testfixtures.SeedBrand(t, tx, uuid.Nil)
	sc := testfixtures.SeedCategory(t, tx)
	mkActiveProduct(t, tx, sb.ID, sc.ID, "Áo Khoác Bomber", 500000, "M", "X")

	repo := NewCatalogPG(tx)
	sugg, err := repo.Suggestions(context.Background(), "ao khoac bombe", 3)
	require.NoError(t, err)
	require.NotEmpty(t, sugg)
}

func TestCatalog_List_Pagination(t *testing.T) {
	tx := testfixtures.BeginTx(t, testPool)
	sb := testfixtures.SeedBrand(t, tx, uuid.Nil)
	sc := testfixtures.SeedCategory(t, tx)
	for i := 0; i < 5; i++ {
		mkActiveProduct(t, tx, sb.ID, sc.ID, "PaginatedProduct", float64(100+i), "M", "X")
	}

	repo := NewCatalogPG(tx)
	// Page 1 with limit 2
	p1, total, err := repo.List(context.Background(), &domain.ListProductsQuery{
		Brand: sb.Slug, Page: 1, Limit: 2, Sort: "newest",
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, total, 5)
	require.Len(t, p1, 2)

	// Page 2
	p2, _, err := repo.List(context.Background(), &domain.ListProductsQuery{
		Brand: sb.Slug, Page: 2, Limit: 2, Sort: "newest",
	})
	require.NoError(t, err)
	require.Len(t, p2, 2)

	// Pages should not overlap
	for _, a := range p1 {
		for _, b := range p2 {
			require.NotEqual(t, a.ID, b.ID)
		}
	}
}
