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
	"github.com/wearwhere/wearwhere_be/internal/wardrobe/domain"
	"github.com/wearwhere/wearwhere_be/internal/wardrobe/repo"
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

func TestClosetPG_ReturnsDeliveredProductsWithTags(t *testing.T) {
	ctx := context.Background()
	tx := testfixtures.BeginTx(t, pool)
	user := testfixtures.SeedCustomer(t, tx)
	brand := testfixtures.SeedBrand(t, tx, uuid.Nil)
	cat := testfixtures.SeedCategory(t, tx)
	tag := testfixtures.SeedStyleTag(t, tx)
	prod := testfixtures.SeedProduct(t, tx, brand.ID, cat.ID, "active")
	variant := testfixtures.SeedVariant(t, tx, prod.ID, "M", "red", 200000, 5)
	_, err := tx.Exec(ctx, `INSERT INTO product_style_tags (product_id, style_tag_id) VALUES ($1,$2)`, prod.ID, tag.ID)
	require.NoError(t, err)
	testfixtures.SeedDeliveredOrderItem(t, tx, user.ID, brand.ID, prod.ID, variant)

	r := repo.NewClosetPG(tx)
	items, err := r.ClosetItems(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, prod.ID, items[0].ProductID)
	require.Equal(t, cat.Slug, items[0].CategorySlug)
	require.Equal(t, []string{tag.Slug}, items[0].StyleSlugs)
}

func TestClosetPG_EmptyWhenNoDeliveries(t *testing.T) {
	ctx := context.Background()
	tx := testfixtures.BeginTx(t, pool)
	user := testfixtures.SeedCustomer(t, tx)
	r := repo.NewClosetPG(tx)
	items, err := r.ClosetItems(ctx, user.ID)
	require.NoError(t, err)
	require.Empty(t, items)
}

func TestSnapshotPG_UpsertAndLoad(t *testing.T) {
	ctx := context.Background()
	tx := testfixtures.BeginTx(t, pool)
	user := testfixtures.SeedCustomer(t, tx)
	r := repo.NewSnapshotPG(tx)

	_, err := r.Load(ctx, user.ID)
	require.ErrorIs(t, err, repo.ErrNoSnapshot)

	outfits := []domain.Outfit{{Title: "Look", Note: "n", ToBuy: []domain.OutfitCard{{ID: uuid.New().String(), Name: "X"}}}}
	require.NoError(t, r.Upsert(ctx, user.ID, "sig1", outfits, "mock", 10, 5))

	snap, err := r.Load(ctx, user.ID)
	require.NoError(t, err)
	require.Equal(t, "sig1", snap.Signature)
	require.Len(t, snap.Outfits, 1)
	require.Equal(t, "Look", snap.Outfits[0].Title)

	require.NoError(t, r.Upsert(ctx, user.ID, "sig2", nil, "mock", 0, 0))
	snap, err = r.Load(ctx, user.ID)
	require.NoError(t, err)
	require.Equal(t, "sig2", snap.Signature)
	require.Empty(t, snap.Outfits)
}
