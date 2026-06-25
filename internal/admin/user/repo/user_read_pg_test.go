//go:build integration

package repo

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/admin/user/domain"
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

func TestListUsers_PaginatesAndCountsTotal(t *testing.T) {
	testfixtures.Clean(t, testPool)
	ctx := context.Background()
	r := NewUserReadPG(testPool)
	for i := 0; i < 3; i++ {
		testfixtures.SeedUser(t, testPool, "customer")
	}

	items, total, err := r.ListUsers(ctx, domain.ListUsersFilter{
		Sort: domain.SortCreatedAt, Order: domain.OrderDesc, Page: 1, PageSize: 2,
	})
	require.NoError(t, err)
	assert.Equal(t, 3, total)   // COUNT(*) OVER() ignores LIMIT
	assert.Len(t, items, 2)     // page capped to 2
}

func TestListUsers_ExcludesDeleted(t *testing.T) {
	testfixtures.Clean(t, testPool)
	ctx := context.Background()
	r := NewUserReadPG(testPool)
	u := testfixtures.SeedUser(t, testPool, "customer")
	_, err := testPool.Exec(ctx,
		`UPDATE users SET status='deleted', deleted_at=NOW() WHERE id=$1`, u.ID)
	require.NoError(t, err)

	_, total, err := r.ListUsers(ctx, domain.ListUsersFilter{
		Sort: domain.SortCreatedAt, Order: domain.OrderDesc, Page: 1, PageSize: 20,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, total)
}

func TestListUsers_SearchByName(t *testing.T) {
	testfixtures.Clean(t, testPool)
	ctx := context.Background()
	r := NewUserReadPG(testPool)
	testfixtures.SeedUser(t, testPool, "customer") // name "Test customer"
	testfixtures.SeedUser(t, testPool, "brand")    // name "Test brand"

	items, total, err := r.ListUsers(ctx, domain.ListUsersFilter{
		Q: "brand", Sort: domain.SortCreatedAt, Order: domain.OrderDesc, Page: 1, PageSize: 20,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, items, 1)
	assert.Equal(t, "brand", items[0].Role)
}

func TestListUsers_EmptyTableReturnsZero(t *testing.T) {
	testfixtures.Clean(t, testPool)
	r := NewUserReadPG(testPool)
	items, total, err := r.ListUsers(context.Background(), domain.ListUsersFilter{
		Sort: domain.SortLastLogin, Order: domain.OrderAsc, Page: 1, PageSize: 20,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, total)
	assert.Len(t, items, 0)
}
