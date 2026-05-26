//go:build integration

package repo_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/cart/repo"
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

func TestCartPG_UpsertIncrementsExistingRow(t *testing.T) {
	tx := testfixtures.BeginTx(t, pool)
	user := testfixtures.SeedCustomer(t, tx)
	brand := testfixtures.SeedBrand(t, tx, uuid.Nil)
	cat := testfixtures.SeedCategory(t, tx)
	prod := testfixtures.SeedProduct(t, tx, brand.ID, cat.ID, "active")
	vid := testfixtures.SeedVariant(t, tx, prod.ID, "M", "Black", 199000, 100)
	r := repo.NewCartPG(tx)

	first, err := r.UpsertAdd(context.Background(), user.ID, vid, 2, 199000)
	require.NoError(t, err)
	require.Equal(t, 2, first.Qty)

	second, err := r.UpsertAdd(context.Background(), user.ID, vid, 3, 199000)
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID)
	require.Equal(t, 5, second.Qty)
}

func TestCartPG_UpsertClampsToTen(t *testing.T) {
	tx := testfixtures.BeginTx(t, pool)
	user := testfixtures.SeedCustomer(t, tx)
	brand := testfixtures.SeedBrand(t, tx, uuid.Nil)
	cat := testfixtures.SeedCategory(t, tx)
	prod := testfixtures.SeedProduct(t, tx, brand.ID, cat.ID, "active")
	vid := testfixtures.SeedVariant(t, tx, prod.ID, "M", "Black", 199000, 100)
	r := repo.NewCartPG(tx)

	_, _ = r.UpsertAdd(context.Background(), user.ID, vid, 8, 199000)
	out, err := r.UpsertAdd(context.Background(), user.ID, vid, 5, 199000)
	require.NoError(t, err)
	require.Equal(t, 10, out.Qty) // clamped
}

func TestCartPG_IDOR_DeleteOtherUserItem(t *testing.T) {
	tx := testfixtures.BeginTx(t, pool)
	owner := testfixtures.SeedCustomer(t, tx)
	intruder := testfixtures.SeedCustomer(t, tx)
	brand := testfixtures.SeedBrand(t, tx, uuid.Nil)
	cat := testfixtures.SeedCategory(t, tx)
	prod := testfixtures.SeedProduct(t, tx, brand.ID, cat.ID, "active")
	vid := testfixtures.SeedVariant(t, tx, prod.ID, "M", "Black", 199000, 5)
	seeded := testfixtures.SeedCartItem(t, tx, owner.ID, vid, 1, 199000)
	r := repo.NewCartPG(tx)

	err := r.Delete(context.Background(), seeded.ID, intruder.ID)
	require.ErrorIs(t, err, repo.ErrNotFound)
}

func TestCartPG_ListView_FlagsSoftDeletedVariant(t *testing.T) {
	tx := testfixtures.BeginTx(t, pool)
	user := testfixtures.SeedCustomer(t, tx)
	brand := testfixtures.SeedBrand(t, tx, uuid.Nil)
	cat := testfixtures.SeedCategory(t, tx)
	prod := testfixtures.SeedProduct(t, tx, brand.ID, cat.ID, "active")
	vid := testfixtures.SeedVariant(t, tx, prod.ID, "M", "Black", 199000, 5)
	testfixtures.SeedCartItem(t, tx, user.ID, vid, 2, 199000)

	// Soft-delete the variant.
	_, err := tx.Exec(context.Background(),
		`UPDATE product_variants SET deleted_at=NOW() WHERE id=$1`, vid)
	require.NoError(t, err)

	r := repo.NewCartPG(tx)
	items, err := r.ListView(context.Background(), user.ID)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.True(t, items[0].Unavailable)
	require.NotNil(t, items[0].UnavailableReason)
	require.Equal(t, "variant_deleted", *items[0].UnavailableReason)
}
