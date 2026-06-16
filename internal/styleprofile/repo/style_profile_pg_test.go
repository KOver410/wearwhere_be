//go:build integration

package repo_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/styleprofile/domain"
	"github.com/wearwhere/wearwhere_be/internal/styleprofile/repo"
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

func intp(v int) *int { return &v }

func TestStyleProfilePG_LoadNotFound(t *testing.T) {
	tx := testfixtures.BeginTx(t, pool)
	user := testfixtures.SeedCustomer(t, tx)
	r := repo.NewStyleProfilePG(tx)

	_, err := r.Load(context.Background(), user.ID)
	require.ErrorIs(t, err, repo.ErrNotFound)
}

func TestStyleProfilePG_UpsertSetsOnboardedAndPreservesIt(t *testing.T) {
	ctx := context.Background()
	tx := testfixtures.BeginTx(t, pool)
	user := testfixtures.SeedCustomer(t, tx)
	tag1 := testfixtures.SeedStyleTag(t, tx)
	tag2 := testfixtures.SeedStyleTag(t, tx)
	r := repo.NewStyleProfilePG(tx)

	v1, err := r.Upsert(ctx, domain.UpsertParams{
		UserID: user.ID, StyleTagIDs: []uuid.UUID{tag1.ID}, BudgetMin: intp(100000), BudgetMax: intp(500000),
	})
	require.NoError(t, err)
	require.NotNil(t, v1.OnboardedAt)
	require.Len(t, v1.StyleTags, 1)
	require.Equal(t, 100000, *v1.BudgetMin)
	firstOnboarded := *v1.OnboardedAt

	v2, err := r.Upsert(ctx, domain.UpsertParams{
		UserID: user.ID, StyleTagIDs: []uuid.UUID{tag2.ID}, BudgetMin: nil, BudgetMax: nil,
	})
	require.NoError(t, err)
	require.Len(t, v2.StyleTags, 1)
	require.Equal(t, tag2.ID.String(), v2.StyleTags[0].ID)
	require.Nil(t, v2.BudgetMin)
	require.WithinDuration(t, firstOnboarded, *v2.OnboardedAt, 0)
}

func TestStyleProfilePG_UnknownTagIDs(t *testing.T) {
	ctx := context.Background()
	tx := testfixtures.BeginTx(t, pool)
	tag := testfixtures.SeedStyleTag(t, tx)
	missing := uuid.New()
	r := repo.NewStyleProfilePG(tx)

	unknown, err := r.UnknownTagIDs(ctx, []uuid.UUID{tag.ID, missing})
	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{missing}, unknown)
}

func TestStyleProfilePG_UpsertWithEmptyTagsClearsAll(t *testing.T) {
	ctx := context.Background()
	tx := testfixtures.BeginTx(t, pool)
	user := testfixtures.SeedCustomer(t, tx)
	tag := testfixtures.SeedStyleTag(t, tx)
	r := repo.NewStyleProfilePG(tx)

	v1, err := r.Upsert(ctx, domain.UpsertParams{UserID: user.ID, StyleTagIDs: []uuid.UUID{tag.ID}})
	require.NoError(t, err)
	require.Len(t, v1.StyleTags, 1)

	// A second upsert with no tags must clear the tag set (not error).
	v2, err := r.Upsert(ctx, domain.UpsertParams{UserID: user.ID, StyleTagIDs: []uuid.UUID{}})
	require.NoError(t, err)
	require.Empty(t, v2.StyleTags)
}
