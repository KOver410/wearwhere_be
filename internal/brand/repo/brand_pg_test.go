//go:build integration

package repo

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/brand/domain"
	"github.com/wearwhere/wearwhere_be/internal/testfixtures"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		panic("TEST_DATABASE_URL not set; run via `make test-integration`")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		panic(err)
	}
	testPool = pool
	code := m.Run()
	pool.Close()
	os.Exit(code)
}

func TestBrandPG_FindByOwnerUserID(t *testing.T) {
	tx := testfixtures.BeginTx(t, testPool)
	sb := testfixtures.SeedBrand(t, tx, uuid.Nil)

	repo := NewBrandPG(tx)
	b, err := repo.FindByOwnerUserID(context.Background(), sb.OwnerID)
	require.NoError(t, err)
	require.Equal(t, sb.ID, b.ID)
	require.Equal(t, sb.Slug, b.Slug)
	require.Equal(t, domain.BrandStatusActive, b.Status)
}

func TestBrandPG_FindByOwnerUserID_NotFound(t *testing.T) {
	tx := testfixtures.BeginTx(t, testPool)
	u := testfixtures.SeedUser(t, tx, "brand") // no brand

	repo := NewBrandPG(tx)
	_, err := repo.FindByOwnerUserID(context.Background(), u.ID)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestBrandPG_Update_Name(t *testing.T) {
	tx := testfixtures.BeginTx(t, testPool)
	sb := testfixtures.SeedBrand(t, tx, uuid.Nil)
	newName := "Updated Name"

	repo := NewBrandPG(tx)
	err := repo.Update(context.Background(), sb.ID,
		&domain.UpdateBrandRequest{Name: &newName})
	require.NoError(t, err)

	b, err := repo.FindByID(context.Background(), sb.ID)
	require.NoError(t, err)
	require.Equal(t, "Updated Name", b.Name)
}

func TestBrandPG_Update_SlugConflict(t *testing.T) {
	tx := testfixtures.BeginTx(t, testPool)
	sb1 := testfixtures.SeedBrand(t, tx, uuid.Nil)
	sb2 := testfixtures.SeedBrand(t, tx, uuid.Nil)

	repo := NewBrandPG(tx)
	err := repo.Update(context.Background(), sb2.ID,
		&domain.UpdateBrandRequest{Slug: &sb1.Slug})
	require.ErrorIs(t, err, ErrSlugTaken)
}

func TestBrandPG_FindBySlug(t *testing.T) {
	tx := testfixtures.BeginTx(t, testPool)
	sb := testfixtures.SeedBrand(t, tx, uuid.Nil)

	repo := NewBrandPG(tx)
	b, err := repo.FindBySlug(context.Background(), sb.Slug)
	require.NoError(t, err)
	require.Equal(t, sb.ID, b.ID)
}

func TestBrandPG_List(t *testing.T) {
	tx := testfixtures.BeginTx(t, testPool)
	for i := 0; i < 3; i++ {
		testfixtures.SeedBrand(t, tx, uuid.Nil)
	}

	repo := NewBrandPG(tx)
	items, total, err := repo.List(context.Background(), "", "newest", 10, 0)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(items), 3)
	require.GreaterOrEqual(t, total, 3)
}

func TestBrandPG_List_WithQueryFilter(t *testing.T) {
	tx := testfixtures.BeginTx(t, testPool)

	// Seed a brand with a fixed name so we can search for it.
	ownerID := testfixtures.SeedUser(t, tx, "brand").ID
	_, err := tx.Exec(context.Background(),
		`INSERT INTO brands (slug, name, owner_user_id, status)
         VALUES ($1, $2, $3, 'active')`,
		"search-target-"+ownerID.String()[:8], "Searchable Brand "+ownerID.String()[:6], ownerID)
	require.NoError(t, err)

	repo := NewBrandPG(tx)
	// Use a fuzzy match. The pg_trgm `%` operator's similarity threshold defaults
	// to ~0.3 so the search term should be similar enough to match.
	items, total, err := repo.List(context.Background(), "Searchable Brand", "newest", 10, 0)
	require.NoError(t, err)
	require.GreaterOrEqual(t, total, 1)
	require.GreaterOrEqual(t, len(items), 1)
}
