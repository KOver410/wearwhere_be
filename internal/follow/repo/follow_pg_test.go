//go:build integration

package repo

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

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

func TestFollowUser_CountAndIdempotent(t *testing.T) {
	testfixtures.Clean(t, testPool)
	ctx := context.Background()
	r := NewFollowPG(testPool)
	a := testfixtures.SeedCustomer(t, testPool)
	b := testfixtures.SeedCustomer(t, testPool)

	c, err := r.FollowUser(ctx, a.ID, b.ID)
	require.NoError(t, err)
	require.Equal(t, 1, c)
	c, err = r.FollowUser(ctx, a.ID, b.ID) // idempotent
	require.NoError(t, err)
	require.Equal(t, 1, c)

	c, err = r.UnfollowUser(ctx, a.ID, b.ID)
	require.NoError(t, err)
	require.Equal(t, 0, c)
	c, err = r.UnfollowUser(ctx, a.ID, b.ID) // idempotent
	require.NoError(t, err)
	require.Equal(t, 0, c)
}

func TestFollowBrand_CountAndList(t *testing.T) {
	testfixtures.Clean(t, testPool)
	ctx := context.Background()
	r := NewFollowPG(testPool)
	u := testfixtures.SeedCustomer(t, testPool)
	sb := testfixtures.SeedBrand(t, testPool, uuid.Nil)

	c, err := r.FollowBrand(ctx, u.ID, sb.ID)
	require.NoError(t, err)
	require.Equal(t, 1, c)

	items, total, err := r.ListFollowingBrands(ctx, u.ID, 20, 0)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, items, 1)
	require.Equal(t, sb.Slug, items[0].Slug)
	require.Equal(t, 1, items[0].FollowerCount)
}

func TestFollowUser_ListAndExists(t *testing.T) {
	testfixtures.Clean(t, testPool)
	ctx := context.Background()
	r := NewFollowPG(testPool)
	a := testfixtures.SeedCustomer(t, testPool)
	b := testfixtures.SeedCustomer(t, testPool)
	require.NoError(t, func() error { _, e := r.FollowUser(ctx, a.ID, b.ID); return e }())

	items, total, err := r.ListFollowingUsers(ctx, a.ID, 20, 0)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Equal(t, b.ID.String(), items[0].ID)

	ok, err := r.UserExists(ctx, b.ID)
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = r.UserExists(ctx, uuid.New())
	require.NoError(t, err)
	require.False(t, ok)
}

func TestSelfFollow_RejectedByDB(t *testing.T) {
	testfixtures.Clean(t, testPool)
	ctx := context.Background()
	r := NewFollowPG(testPool)
	a := testfixtures.SeedCustomer(t, testPool)
	_, err := r.FollowUser(ctx, a.ID, a.ID) // violates CHECK (follower_id <> followee_id)
	require.Error(t, err)
}
