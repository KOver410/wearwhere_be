//go:build integration

package repo

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/product/domain"
	"github.com/wearwhere/wearwhere_be/internal/testfixtures"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		panic("TEST_DATABASE_URL not set")
	}
	p, err := pgxpool.New(context.Background(), url)
	if err != nil {
		panic(err)
	}
	testPool = p
	code := m.Run()
	p.Close()
	os.Exit(code)
}

func TestProductPG_Create(t *testing.T) {
	tx := testfixtures.BeginTx(t, testPool)
	sb := testfixtures.SeedBrand(t, tx, uuid.Nil)
	sc := testfixtures.SeedCategory(t, tx)

	repo := NewProductPG(tx)
	p, err := repo.Create(context.Background(), sb.ID, "my-slug",
		&domain.CreateProductRequest{
			Name: "My Product", CategoryID: sc.ID.String(),
		})
	require.NoError(t, err)
	require.Equal(t, "my-slug", p.Slug)
	require.Equal(t, domain.ProductStatusDraft, p.Status)
}

func TestProductPG_SlugExists_ScopedByBrand(t *testing.T) {
	tx := testfixtures.BeginTx(t, testPool)
	sb1 := testfixtures.SeedBrand(t, tx, uuid.Nil)
	sb2 := testfixtures.SeedBrand(t, tx, uuid.Nil)
	sc := testfixtures.SeedCategory(t, tx)
	ctx := context.Background()

	repo := NewProductPG(tx)
	_, err := repo.Create(ctx, sb1.ID, "shared-slug",
		&domain.CreateProductRequest{Name: "P1", CategoryID: sc.ID.String()})
	require.NoError(t, err)

	// Same slug under a different brand is OK
	_, err = repo.Create(ctx, sb2.ID, "shared-slug",
		&domain.CreateProductRequest{Name: "P2", CategoryID: sc.ID.String()})
	require.NoError(t, err)

	// Within same brand, returns ErrSlugTaken
	_, err = repo.Create(ctx, sb1.ID, "shared-slug",
		&domain.CreateProductRequest{Name: "P3", CategoryID: sc.ID.String()})
	require.ErrorIs(t, err, ErrSlugTaken)
}

func TestProductPG_Update_IDORProtected(t *testing.T) {
	tx := testfixtures.BeginTx(t, testPool)
	sbA := testfixtures.SeedBrand(t, tx, uuid.Nil)
	sbB := testfixtures.SeedBrand(t, tx, uuid.Nil)
	sc := testfixtures.SeedCategory(t, tx)
	p := testfixtures.SeedProduct(t, tx, sbA.ID, sc.ID, string(domain.ProductStatusDraft))

	repo := NewProductPG(tx)
	newName := "Hacker rename"
	err := repo.Update(context.Background(), p.ID, sbB.ID,
		&domain.UpdateProductRequest{Name: &newName})
	require.ErrorIs(t, err, ErrNotFound)

	// brand A can still update
	require.NoError(t, repo.Update(context.Background(), p.ID, sbA.ID,
		&domain.UpdateProductRequest{Name: &newName}))
}

func TestProductPG_SearchTriggerUpdatesOnBrandRename(t *testing.T) {
	tx := testfixtures.BeginTx(t, testPool)
	sb := testfixtures.SeedBrand(t, tx, uuid.Nil)
	sc := testfixtures.SeedCategory(t, tx)
	_ = testfixtures.SeedProduct(t, tx, sb.ID, sc.ID, string(domain.ProductStatusActive))

	var initial string
	require.NoError(t, tx.QueryRow(context.Background(),
		`SELECT search_text FROM products WHERE brand_id=$1 LIMIT 1`, sb.ID).Scan(&initial))
	require.Contains(t, initial, "brand")

	// Rename the brand
	_, err := tx.Exec(context.Background(),
		`UPDATE brands SET name = $1 WHERE id = $2`, "Hoàn Toàn Mới", sb.ID)
	require.NoError(t, err)

	var updated string
	require.NoError(t, tx.QueryRow(context.Background(),
		`SELECT search_text FROM products WHERE brand_id=$1 LIMIT 1`, sb.ID).Scan(&updated))
	require.Contains(t, updated, "hoan toan moi") // unaccented
}

func TestProductPG_StyleTagsSync(t *testing.T) {
	tx := testfixtures.BeginTx(t, testPool)
	sb := testfixtures.SeedBrand(t, tx, uuid.Nil)
	sc := testfixtures.SeedCategory(t, tx)
	st1 := testfixtures.SeedStyleTag(t, tx)
	st2 := testfixtures.SeedStyleTag(t, tx)
	p := testfixtures.SeedProduct(t, tx, sb.ID, sc.ID, "draft")

	repo := NewProductPG(tx)
	ctx := context.Background()

	require.NoError(t, repo.SetStyleTags(ctx, p.ID, []uuid.UUID{st1.ID, st2.ID}))
	tags, err := repo.GetStyleTags(ctx, p.ID)
	require.NoError(t, err)
	require.Len(t, tags, 2)

	require.NoError(t, repo.SetStyleTags(ctx, p.ID, []uuid.UUID{st1.ID}))
	tags, err = repo.GetStyleTags(ctx, p.ID)
	require.NoError(t, err)
	require.Len(t, tags, 1)
}
