//go:build integration

package repo_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/testfixtures"
	"github.com/wearwhere/wearwhere_be/internal/wishlist/repo"
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

func TestWishlistPG_AddRemove_Idempotent(t *testing.T) {
	tx := testfixtures.BeginTx(t, pool)
	user := testfixtures.SeedCustomer(t, tx)
	brand := testfixtures.SeedBrand(t, tx, uuid.Nil)
	cat := testfixtures.SeedCategory(t, tx)
	prod := testfixtures.SeedProduct(t, tx, brand.ID, cat.ID, "active")
	r := repo.NewWishlistPG(tx)

	require.NoError(t, r.Add(context.Background(), user.ID, prod.ID))
	// Second add must not error.
	require.NoError(t, r.Add(context.Background(), user.ID, prod.ID))

	require.NoError(t, r.Remove(context.Background(), user.ID, prod.ID))
	// Second remove must not error.
	require.NoError(t, r.Remove(context.Background(), user.ID, prod.ID))
}

func TestWishlistPG_List_ExcludesInactiveProducts(t *testing.T) {
	tx := testfixtures.BeginTx(t, pool)
	user := testfixtures.SeedCustomer(t, tx)
	brand := testfixtures.SeedBrand(t, tx, uuid.Nil)
	cat := testfixtures.SeedCategory(t, tx)
	active := testfixtures.SeedProduct(t, tx, brand.ID, cat.ID, "active")
	draft := testfixtures.SeedProduct(t, tx, brand.ID, cat.ID, "draft")
	testfixtures.SeedWishlistItem(t, tx, user.ID, active.ID)
	testfixtures.SeedWishlistItem(t, tx, user.ID, draft.ID)
	r := repo.NewWishlistPG(tx)

	items, total, err := r.List(context.Background(), user.ID, 20, 0)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, items, 1)
	require.Equal(t, active.ID, items[0].ProductID)
}

func TestWishlistPG_Contains_MixedHitMiss(t *testing.T) {
	tx := testfixtures.BeginTx(t, pool)
	user := testfixtures.SeedCustomer(t, tx)
	brand := testfixtures.SeedBrand(t, tx, uuid.Nil)
	cat := testfixtures.SeedCategory(t, tx)
	a := testfixtures.SeedProduct(t, tx, brand.ID, cat.ID, "active")
	b := testfixtures.SeedProduct(t, tx, brand.ID, cat.ID, "active")
	testfixtures.SeedWishlistItem(t, tx, user.ID, a.ID)
	r := repo.NewWishlistPG(tx)

	res, err := r.Contains(context.Background(), user.ID, []uuid.UUID{a.ID, b.ID})
	require.NoError(t, err)
	require.True(t, res[a.ID])
	require.False(t, res[b.ID]) // absent → false (zero value)
}
