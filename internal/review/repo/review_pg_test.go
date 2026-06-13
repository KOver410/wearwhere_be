//go:build integration

package repo

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/review/domain"
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

// seedProduct creates an active brand + product + variant against the pool (committed).
func seedProduct(t *testing.T) (brand, product, variant uuid.UUID) {
	t.Helper()
	sb := testfixtures.SeedBrand(t, testPool, uuid.Nil)
	cat := testfixtures.SeedCategory(t, testPool)
	p := testfixtures.SeedProduct(t, testPool, sb.ID, cat.ID, "active")
	v := testfixtures.SeedVariant(t, testPool, p.ID, "M", "Black", 100000, 10)
	return sb.ID, p.ID, v
}

func TestReviewPG_Create_RecomputesAggregate(t *testing.T) {
	testfixtures.Clean(t, testPool)
	ctx := context.Background()
	brand, product, variant := seedProduct(t)
	user := testfixtures.SeedCustomer(t, testPool)
	testfixtures.SeedDeliveredOrderItem(t, testPool, user.ID, brand, product, variant)

	r := NewReviewPG(testPool)
	rv := &domain.Review{ProductID: product, UserID: user.ID, Rating: 4, Body: "Great fit and quality, very happy"}
	require.NoError(t, r.Create(ctx, rv))
	require.NotEqual(t, uuid.Nil, rv.ID)

	agg, err := r.Aggregate(ctx, product)
	require.NoError(t, err)
	require.Equal(t, 1, agg.ReviewCount)
	require.InDelta(t, 4.0, agg.AvgRating, 0.01)
}

func TestReviewPG_Create_DuplicateReturnsErrDuplicate(t *testing.T) {
	testfixtures.Clean(t, testPool)
	ctx := context.Background()
	brand, product, variant := seedProduct(t)
	user := testfixtures.SeedCustomer(t, testPool)
	testfixtures.SeedDeliveredOrderItem(t, testPool, user.ID, brand, product, variant)
	r := NewReviewPG(testPool)
	require.NoError(t, r.Create(ctx, &domain.Review{ProductID: product, UserID: user.ID, Rating: 5, Body: "First review body twenty chars"}))
	err := r.Create(ctx, &domain.Review{ProductID: product, UserID: user.ID, Rating: 3, Body: "Second review should be blocked"})
	require.ErrorIs(t, err, ErrDuplicate)
}

func TestReviewPG_HasDeliveredPurchase(t *testing.T) {
	testfixtures.Clean(t, testPool)
	ctx := context.Background()
	brand, product, variant := seedProduct(t)
	user := testfixtures.SeedCustomer(t, testPool)
	r := NewReviewPG(testPool)

	ok, err := r.HasDeliveredPurchase(ctx, user.ID, product)
	require.NoError(t, err)
	require.False(t, ok, "no delivered purchase yet")

	testfixtures.SeedDeliveredOrderItem(t, testPool, user.ID, brand, product, variant)
	ok, err = r.HasDeliveredPurchase(ctx, user.ID, product)
	require.NoError(t, err)
	require.True(t, ok, "delivered purchase exists")
}

func TestReviewPG_ListByProduct_FilterAndSort(t *testing.T) {
	testfixtures.Clean(t, testPool)
	ctx := context.Background()
	brand, product, variant := seedProduct(t)
	u1 := testfixtures.SeedCustomer(t, testPool)
	u2 := testfixtures.SeedCustomer(t, testPool)
	testfixtures.SeedDeliveredOrderItem(t, testPool, u1.ID, brand, product, variant)
	testfixtures.SeedDeliveredOrderItem(t, testPool, u2.ID, brand, product, variant)
	r := NewReviewPG(testPool)
	require.NoError(t, r.Create(ctx, &domain.Review{ProductID: product, UserID: u1.ID, Rating: 5, Body: "Five star review body text!!"}))
	require.NoError(t, r.Create(ctx, &domain.Review{ProductID: product, UserID: u2.ID, Rating: 3, Body: "Three star review body text!!"}))

	// rating filter
	got, total, err := r.ListByProduct(ctx, product, ListFilter{Rating: 5, Limit: 20, Offset: 0})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, got, 1)
	require.Equal(t, 5, got[0].Rating)
	require.NotEmpty(t, got[0].ReviewerName)

	// sort rating_low → 3 before 5
	got, total, err = r.ListByProduct(ctx, product, ListFilter{Sort: "rating_low", Limit: 20, Offset: 0})
	require.NoError(t, err)
	require.Equal(t, 2, total)
	require.Equal(t, 3, got[0].Rating)
	require.Equal(t, 5, got[1].Rating)
}

func TestReviewPG_SoftDelete_UpdatesAggregate(t *testing.T) {
	testfixtures.Clean(t, testPool)
	ctx := context.Background()
	brand, product, variant := seedProduct(t)
	user := testfixtures.SeedCustomer(t, testPool)
	testfixtures.SeedDeliveredOrderItem(t, testPool, user.ID, brand, product, variant)
	r := NewReviewPG(testPool)
	rv := &domain.Review{ProductID: product, UserID: user.ID, Rating: 4, Body: "Review to be soft-deleted soon"}
	require.NoError(t, r.Create(ctx, rv))

	require.NoError(t, r.SoftDelete(ctx, rv.ID))
	agg, err := r.Aggregate(ctx, product)
	require.NoError(t, err)
	require.Equal(t, 0, agg.ReviewCount)
	require.InDelta(t, 0.0, agg.AvgRating, 0.01)

	// soft-deleting again → ErrNotFound
	require.ErrorIs(t, r.SoftDelete(ctx, rv.ID), ErrNotFound)
}
