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

func TestVariantPG_Create(t *testing.T) {
	tx := testfixtures.BeginTx(t, testPool)
	sb := testfixtures.SeedBrand(t, tx, uuid.Nil)
	sc := testfixtures.SeedCategory(t, tx)
	p := testfixtures.SeedProduct(t, tx, sb.ID, sc.ID, "draft")

	repo := NewVariantPG(tx)
	v, err := repo.Create(context.Background(), p.ID, &domain.CreateVariantRequest{
		SKU: "SKU-A", Size: "M", Color: "White", Price: 250000, StockQty: 5,
	})
	require.NoError(t, err)
	require.Equal(t, p.ID, v.ProductID)
	require.True(t, v.IsActive)
}

func TestVariantPG_DuplicateSizeColor_Conflict(t *testing.T) {
	tx := testfixtures.BeginTx(t, testPool)
	sb := testfixtures.SeedBrand(t, tx, uuid.Nil)
	sc := testfixtures.SeedCategory(t, tx)
	p := testfixtures.SeedProduct(t, tx, sb.ID, sc.ID, "draft")

	repo := NewVariantPG(tx)
	ctx := context.Background()
	_, err := repo.Create(ctx, p.ID, &domain.CreateVariantRequest{
		SKU: "SKU-A", Size: "M", Color: "White", Price: 100, StockQty: 0,
	})
	require.NoError(t, err)
	_, err = repo.Create(ctx, p.ID, &domain.CreateVariantRequest{
		SKU: "SKU-B", Size: "M", Color: "White", Price: 200, StockQty: 0,
	})
	require.ErrorIs(t, err, ErrVariantConflict)
}

func TestVariantPG_IDORProtected(t *testing.T) {
	tx := testfixtures.BeginTx(t, testPool)
	sb := testfixtures.SeedBrand(t, tx, uuid.Nil)
	sc := testfixtures.SeedCategory(t, tx)
	pA := testfixtures.SeedProduct(t, tx, sb.ID, sc.ID, "draft")
	pB := testfixtures.SeedProduct(t, tx, sb.ID, sc.ID, "draft")
	vID := testfixtures.SeedVariant(t, tx, pA.ID, "S", "Red", 100, 1)

	repo := NewVariantPG(tx)
	_, err := repo.FindByID(context.Background(), vID, pB.ID)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestVariantPG_Update(t *testing.T) {
	tx := testfixtures.BeginTx(t, testPool)
	sb := testfixtures.SeedBrand(t, tx, uuid.Nil)
	sc := testfixtures.SeedCategory(t, tx)
	p := testfixtures.SeedProduct(t, tx, sb.ID, sc.ID, "draft")
	vID := testfixtures.SeedVariant(t, tx, p.ID, "M", "White", 100, 5)

	repo := NewVariantPG(tx)
	newPrice := 999.0
	newStock := 50
	v, err := repo.Update(context.Background(), vID, p.ID, &domain.UpdateVariantRequest{
		Price: &newPrice, StockQty: &newStock,
	})
	require.NoError(t, err)
	require.Equal(t, 999.0, v.Price)
	require.Equal(t, 50, v.StockQty)
}

func TestVariantPG_Update_IDORProtected(t *testing.T) {
	tx := testfixtures.BeginTx(t, testPool)
	sb := testfixtures.SeedBrand(t, tx, uuid.Nil)
	sc := testfixtures.SeedCategory(t, tx)
	pA := testfixtures.SeedProduct(t, tx, sb.ID, sc.ID, "draft")
	pB := testfixtures.SeedProduct(t, tx, sb.ID, sc.ID, "draft")
	vID := testfixtures.SeedVariant(t, tx, pA.ID, "L", "Blue", 200, 2)

	repo := NewVariantPG(tx)
	newPrice := 999.0
	_, err := repo.Update(context.Background(), vID, pB.ID, &domain.UpdateVariantRequest{
		Price: &newPrice,
	})
	require.ErrorIs(t, err, ErrNotFound)
}

func TestVariantPG_SoftDelete(t *testing.T) {
	tx := testfixtures.BeginTx(t, testPool)
	sb := testfixtures.SeedBrand(t, tx, uuid.Nil)
	sc := testfixtures.SeedCategory(t, tx)
	p := testfixtures.SeedProduct(t, tx, sb.ID, sc.ID, "draft")
	vID := testfixtures.SeedVariant(t, tx, p.ID, "XL", "Black", 300, 3)

	repo := NewVariantPG(tx)
	ctx := context.Background()

	// soft-delete succeeds
	require.NoError(t, repo.SoftDelete(ctx, vID, p.ID))

	// second delete returns ErrNotFound (already soft-deleted)
	err := repo.SoftDelete(ctx, vID, p.ID)
	require.ErrorIs(t, err, ErrNotFound)

	// FindByID also returns ErrNotFound after soft-delete
	_, err = repo.FindByID(ctx, vID, p.ID)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestVariantPG_SoftDelete_IDORProtected(t *testing.T) {
	tx := testfixtures.BeginTx(t, testPool)
	sb := testfixtures.SeedBrand(t, tx, uuid.Nil)
	sc := testfixtures.SeedCategory(t, tx)
	pA := testfixtures.SeedProduct(t, tx, sb.ID, sc.ID, "draft")
	pB := testfixtures.SeedProduct(t, tx, sb.ID, sc.ID, "draft")
	vID := testfixtures.SeedVariant(t, tx, pA.ID, "S", "Green", 150, 1)

	repo := NewVariantPG(tx)
	err := repo.SoftDelete(context.Background(), vID, pB.ID)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestVariantPG_ListByProduct_ActiveOnlyFilter(t *testing.T) {
	tx := testfixtures.BeginTx(t, testPool)
	sb := testfixtures.SeedBrand(t, tx, uuid.Nil)
	sc := testfixtures.SeedCategory(t, tx)
	p := testfixtures.SeedProduct(t, tx, sb.ID, sc.ID, "draft")

	repo := NewVariantPG(tx)
	ctx := context.Background()

	// Create two variants: one active (default), one inactive
	active, err := repo.Create(ctx, p.ID, &domain.CreateVariantRequest{
		SKU: "SKU-ACTIVE", Size: "M", Color: "Red", Price: 100, StockQty: 1,
	})
	require.NoError(t, err)

	inactive, err := repo.Create(ctx, p.ID, &domain.CreateVariantRequest{
		SKU: "SKU-INACTIVE", Size: "L", Color: "Blue", Price: 200, StockQty: 0,
	})
	require.NoError(t, err)

	// Mark the second one inactive
	f := false
	_, err = repo.Update(ctx, inactive.ID, p.ID, &domain.UpdateVariantRequest{IsActive: &f})
	require.NoError(t, err)

	// onlyActive=false → both returned
	all, err := repo.ListByProduct(ctx, p.ID, false)
	require.NoError(t, err)
	require.Len(t, all, 2)

	// onlyActive=true → only the active one
	activeOnly, err := repo.ListByProduct(ctx, p.ID, true)
	require.NoError(t, err)
	require.Len(t, activeOnly, 1)
	require.Equal(t, active.ID, activeOnly[0].ID)
}

func TestVariantPG_ListByProduct_ExcludesSoftDeleted(t *testing.T) {
	tx := testfixtures.BeginTx(t, testPool)
	sb := testfixtures.SeedBrand(t, tx, uuid.Nil)
	sc := testfixtures.SeedCategory(t, tx)
	p := testfixtures.SeedProduct(t, tx, sb.ID, sc.ID, "draft")

	repo := NewVariantPG(tx)
	ctx := context.Background()

	v1, err := repo.Create(ctx, p.ID, &domain.CreateVariantRequest{
		SKU: "SKU-1", Size: "S", Color: "White", Price: 100, StockQty: 1,
	})
	require.NoError(t, err)

	_, err = repo.Create(ctx, p.ID, &domain.CreateVariantRequest{
		SKU: "SKU-2", Size: "M", Color: "White", Price: 200, StockQty: 2,
	})
	require.NoError(t, err)

	// Soft-delete the first variant
	require.NoError(t, repo.SoftDelete(ctx, v1.ID, p.ID))

	// List should only return the non-deleted one
	list, err := repo.ListByProduct(ctx, p.ID, false)
	require.NoError(t, err)
	require.Len(t, list, 1)
}
