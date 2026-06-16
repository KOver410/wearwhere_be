//go:build integration

package repo_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/recommendation/repo"
	"github.com/wearwhere/wearwhere_be/internal/testfixtures"
)

var pool *pgxpool.Pool

func TestMain(m *testing.M) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		panic("TEST_DATABASE_URL required")
	}
	var err error
	pool, err = pgxpool.New(context.Background(), dsn)
	if err != nil {
		panic(err)
	}
	defer pool.Close()
	os.Exit(m.Run())
}

func TestCandidatePG_ReturnsActiveInStockWithTags(t *testing.T) {
	ctx := context.Background()
	tx := testfixtures.BeginTx(t, pool)
	brand := testfixtures.SeedBrand(t, tx, uuid.Nil)
	cat := testfixtures.SeedCategory(t, tx)
	tag := testfixtures.SeedStyleTag(t, tx)

	inStock := testfixtures.SeedProduct(t, tx, brand.ID, cat.ID, "active")
	testfixtures.SeedVariant(t, tx, inStock.ID, "M", "red", 200000, 5)
	_, err := tx.Exec(ctx, `INSERT INTO product_style_tags (product_id, style_tag_id) VALUES ($1,$2)`, inStock.ID, tag.ID)
	require.NoError(t, err)

	oos := testfixtures.SeedProduct(t, tx, brand.ID, cat.ID, "active")
	testfixtures.SeedVariant(t, tx, oos.ID, "M", "blue", 150000, 0)

	r := repo.NewCandidatePG(tx)
	cands, err := r.Candidates(ctx, 100)
	require.NoError(t, err)

	var found bool
	for _, c := range cands {
		require.NotEqual(t, oos.ID, c.ProductID, "out-of-stock product must be excluded")
		if c.ProductID == inStock.ID {
			found = true
			require.Equal(t, []uuid.UUID{tag.ID}, c.StyleTagIDs)
			require.Equal(t, float64(200000), c.MinPrice)
			require.Equal(t, brand.Slug, c.BrandSlug)
		}
	}
	require.True(t, found, "in-stock tagged product must be returned")
}

func TestSignalPG_FollowedBrands_Purchases_Affinity(t *testing.T) {
	ctx := context.Background()
	tx := testfixtures.BeginTx(t, pool)
	user := testfixtures.SeedCustomer(t, tx)
	brand := testfixtures.SeedBrand(t, tx, uuid.Nil)
	cat := testfixtures.SeedCategory(t, tx)
	prod := testfixtures.SeedProduct(t, tx, brand.ID, cat.ID, "active")
	variant := testfixtures.SeedVariant(t, tx, prod.ID, "M", "red", 200000, 5)

	_, err := tx.Exec(ctx, `INSERT INTO brand_follows (user_id, brand_id) VALUES ($1,$2)`, user.ID, brand.ID)
	require.NoError(t, err)
	testfixtures.SeedDeliveredOrderItem(t, tx, user.ID, brand.ID, prod.ID, variant)

	sr := repo.NewSignalPG(tx)

	brands, err := sr.FollowedBrandIDs(ctx, user.ID)
	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{brand.ID}, brands)

	purchased, err := sr.PurchasedProductIDs(ctx, user.ID)
	require.NoError(t, err)
	require.Contains(t, purchased, prod.ID)

	cats, err := sr.AffinityCategoryIDs(ctx, user.ID)
	require.NoError(t, err)
	require.Contains(t, cats, cat.ID)
}
