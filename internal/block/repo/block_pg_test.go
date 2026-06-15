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

func TestBlock_Idempotent_AndList(t *testing.T) {
	testfixtures.Clean(t, testPool)
	ctx := context.Background()
	r := NewBlockPG(testPool)
	a := testfixtures.SeedCustomer(t, testPool)
	b := testfixtures.SeedCustomer(t, testPool)

	require.NoError(t, r.Block(ctx, a.ID, b.ID))
	require.NoError(t, r.Block(ctx, a.ID, b.ID)) // idempotent, no error

	items, total, err := r.ListBlocked(ctx, a.ID, 20, 0)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, items, 1)
	require.Equal(t, b.ID.String(), items[0].ID)

	require.NoError(t, r.Unblock(ctx, a.ID, b.ID))
	require.NoError(t, r.Unblock(ctx, a.ID, b.ID)) // idempotent
	_, total, err = r.ListBlocked(ctx, a.ID, 20, 0)
	require.NoError(t, err)
	require.Equal(t, 0, total)
}

func TestUserExists(t *testing.T) {
	testfixtures.Clean(t, testPool)
	ctx := context.Background()
	r := NewBlockPG(testPool)
	a := testfixtures.SeedCustomer(t, testPool)

	ok, err := r.UserExists(ctx, a.ID)
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = r.UserExists(ctx, uuid.New())
	require.NoError(t, err)
	require.False(t, ok)
}

func TestSelfBlock_RejectedByDB(t *testing.T) {
	testfixtures.Clean(t, testPool)
	ctx := context.Background()
	r := NewBlockPG(testPool)
	a := testfixtures.SeedCustomer(t, testPool)
	err := r.Block(ctx, a.ID, a.ID) // violates CHECK (blocker_id <> blocked_id)
	require.Error(t, err)
}
