//go:build integration

package repo

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/testfixtures"
)

// seedVariantForReserve seeds a brand, category, product, and variant,
// returning the variantID. Uses a BeginTx so data rolls back after the test.
// For the race test we need a real commit, so it accepts a DBTX (can be pool or tx).
func seedVariantForReserve(t *testing.T, db testfixtures.DBTX, stockQty int) uuid.UUID {
	t.Helper()
	sb := testfixtures.SeedBrand(t, db, uuid.Nil)
	sc := testfixtures.SeedCategory(t, db)
	p := testfixtures.SeedProduct(t, db, sb.ID, sc.ID, "active")
	return testfixtures.SeedVariant(t, db, p.ID, "M", "Black", 100000, stockQty)
}

// getVariantStock reads stock_qty and reserved_qty directly.
func getVariantStock(t *testing.T, db testfixtures.DBTX, variantID uuid.UUID) (stock, reserved int) {
	t.Helper()
	err := db.QueryRow(context.Background(),
		`SELECT stock_qty, reserved_qty FROM product_variants WHERE id=$1`,
		variantID).Scan(&stock, &reserved)
	require.NoError(t, err)
	return
}

func TestVariantReserve_Success(t *testing.T) {
	tx := testfixtures.BeginTx(t, testPool)
	variantID := seedVariantForReserve(t, tx, 10)

	repo := NewVariantPG(tx)
	ctx := context.Background()

	require.NoError(t, repo.Reserve(ctx, tx, variantID, 3))

	stock, reserved := getVariantStock(t, tx, variantID)
	require.Equal(t, 10, stock)
	require.Equal(t, 3, reserved)
}

func TestVariantReserve_InsufficientStock(t *testing.T) {
	tx := testfixtures.BeginTx(t, testPool)
	variantID := seedVariantForReserve(t, tx, 2)

	repo := NewVariantPG(tx)
	ctx := context.Background()

	err := repo.Reserve(ctx, tx, variantID, 5)
	require.ErrorIs(t, err, ErrInsufficientStock)
}

func TestVariantCommit_DecrementsBoth(t *testing.T) {
	tx := testfixtures.BeginTx(t, testPool)
	variantID := seedVariantForReserve(t, tx, 10)

	repo := NewVariantPG(tx)
	ctx := context.Background()

	require.NoError(t, repo.Reserve(ctx, tx, variantID, 3))
	require.NoError(t, repo.Commit(ctx, tx, variantID, 3))

	stock, reserved := getVariantStock(t, tx, variantID)
	require.Equal(t, 7, stock)
	require.Equal(t, 0, reserved)
}

func TestVariantRelease_DecrementsReservedOnly(t *testing.T) {
	tx := testfixtures.BeginTx(t, testPool)
	variantID := seedVariantForReserve(t, tx, 10)

	repo := NewVariantPG(tx)
	ctx := context.Background()

	require.NoError(t, repo.Reserve(ctx, tx, variantID, 4))
	require.NoError(t, repo.Release(ctx, tx, variantID, 4))

	stock, reserved := getVariantStock(t, tx, variantID)
	require.Equal(t, 10, stock)
	require.Equal(t, 0, reserved)
}

// TestVariantReserve_ConcurrentRace tests that with 1 unit and 2 concurrent
// reserves of 1, exactly one succeeds. Needs real commits so uses pool directly
// and cleans up afterward.
func TestVariantReserve_ConcurrentRace(t *testing.T) {
	ctx := context.Background()

	// Seed via a committed transaction (pool directly, then clean up with defer).
	sb := testfixtures.SeedBrand(t, testPool, uuid.Nil)
	sc := testfixtures.SeedCategory(t, testPool)
	p := testfixtures.SeedProduct(t, testPool, sb.ID, sc.ID, "active")
	variantID := testfixtures.SeedVariant(t, testPool, p.ID, "M", "Black", 100000, 1)
	t.Cleanup(func() {
		_, _ = testPool.Exec(ctx, `DELETE FROM product_variants WHERE id=$1`, variantID)
		_, _ = testPool.Exec(ctx, `DELETE FROM products WHERE id=$1`, p.ID)
		_, _ = testPool.Exec(ctx, `DELETE FROM brands WHERE id=$1`, sb.ID)
	})

	repo := NewVariantPG(testPool)

	var wg sync.WaitGroup
	results := make([]error, 2)
	for i := 0; i < 2; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = repo.Reserve(ctx, testPool, variantID, 1)
		}()
	}
	wg.Wait()

	success := 0
	for _, e := range results {
		if e == nil {
			success++
		}
	}
	require.Equal(t, 1, success, "exactly one of the 2 reserves should succeed")
}
