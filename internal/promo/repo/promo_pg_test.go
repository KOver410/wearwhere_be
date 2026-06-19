//go:build integration

package repo_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/promo/domain"
	"github.com/wearwhere/wearwhere_be/internal/promo/repo"
	"github.com/wearwhere/wearwhere_be/internal/testfixtures"
)

func newRepo(pool *pgxpool.Pool) *repo.PromoPG { return repo.NewPromoPG(pool) }

func TestPromoPG_CreateAndGetActive(t *testing.T) {
	pool := testfixtures.MustPool(t)
	r := newRepo(pool)
	ctx := context.Background()

	p := &domain.PromoCode{
		Code:          "SAVE10",
		DiscountType:  domain.DiscountTypePercentage,
		DiscountValue: 10,
		StartsAt:      time.Now().Add(-time.Hour),
		IsActive:      true,
	}
	require.NoError(t, r.Create(ctx, p))
	require.NotEqual(t, uuid.Nil, p.ID)

	got, err := r.GetActiveByCode(ctx, nil, "save10") // CITEXT → case-insensitive
	require.NoError(t, err)
	assert.Equal(t, p.ID, got.ID)
	assert.Equal(t, domain.DiscountTypePercentage, got.DiscountType)
}

func TestPromoPG_Create_DuplicateConflict(t *testing.T) {
	pool := testfixtures.MustPool(t)
	r := newRepo(pool)
	ctx := context.Background()

	p1 := &domain.PromoCode{Code: "DUPC", DiscountType: domain.DiscountTypeFixed, DiscountValue: 1000, StartsAt: time.Now(), IsActive: true}
	require.NoError(t, r.Create(ctx, p1))
	p2 := &domain.PromoCode{Code: "dupc", DiscountType: domain.DiscountTypeFixed, DiscountValue: 1000, StartsAt: time.Now(), IsActive: true}
	assert.ErrorIs(t, r.Create(ctx, p2), repo.ErrCodeConflict)
}

func TestPromoPG_GetActive_InactiveIsNotFound(t *testing.T) {
	pool := testfixtures.MustPool(t)
	r := newRepo(pool)
	ctx := context.Background()

	p := &domain.PromoCode{Code: "OFF1", DiscountType: domain.DiscountTypeFixed, DiscountValue: 1000, StartsAt: time.Now(), IsActive: false}
	require.NoError(t, r.Create(ctx, p))
	_, err := r.GetActiveByCode(ctx, nil, "OFF1")
	assert.ErrorIs(t, err, repo.ErrNotFound)
}

func TestPromoPG_Redemption_UniquePerUser(t *testing.T) {
	pool := testfixtures.MustPool(t)
	r := newRepo(pool)
	ctx := context.Background()

	user := testfixtures.SeedCustomer(t, pool)
	p := &domain.PromoCode{Code: "ONEUSE", DiscountType: domain.DiscountTypeFixed, DiscountValue: 5000, StartsAt: time.Now(), IsActive: true}
	require.NoError(t, r.Create(ctx, p))

	// A redemption needs a real order row (FK). Seed a minimal order.
	orderID := seedBareOrder(t, pool, user.ID)

	require.NoError(t, r.InsertRedemption(ctx, nil, p.ID, user.ID, orderID, 5000))

	has, err := r.HasRedeemed(ctx, nil, p.ID, user.ID)
	require.NoError(t, err)
	assert.True(t, has)

	// Second redemption by same user → conflict.
	orderID2 := seedBareOrder(t, pool, user.ID)
	err = r.InsertRedemption(ctx, nil, p.ID, user.ID, orderID2, 5000)
	assert.ErrorIs(t, err, repo.ErrAlreadyRedeemed)
}

func TestPromoPG_List(t *testing.T) {
	pool := testfixtures.MustPool(t)
	r := newRepo(pool)
	ctx := context.Background()

	require.NoError(t, r.Create(ctx, &domain.PromoCode{Code: "L1", DiscountType: domain.DiscountTypeFixed, DiscountValue: 1000, StartsAt: time.Now(), IsActive: true}))
	require.NoError(t, r.Create(ctx, &domain.PromoCode{Code: "L2", DiscountType: domain.DiscountTypeFixed, DiscountValue: 2000, StartsAt: time.Now(), IsActive: false}))

	all, total, err := r.List(ctx, 1, 50, false)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, total, 2)
	assert.GreaterOrEqual(t, len(all), 2)

	active, _, err := r.List(ctx, 1, 50, true)
	require.NoError(t, err)
	for _, p := range active {
		assert.True(t, p.IsActive)
	}
}

// seedBareOrder inserts a minimal order row to satisfy the redemption FK.
func seedBareOrder(t *testing.T, pool *pgxpool.Pool, userID uuid.UUID) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	orderNo := "PROMO-" + uuid.NewString()[:8]
	err := pool.QueryRow(context.Background(),
		`INSERT INTO orders
		   (user_id, order_no, subtotal_vnd, shipping_total_vnd, grand_total_vnd,
		    payment_method, payment_status, status, shipping_address)
		 VALUES ($1, $2, 100000, 0, 100000, 'cod', 'pending', 'processing', '{}'::jsonb)
		 RETURNING id`, userID, orderNo).Scan(&id)
	require.NoError(t, err)
	return id
}
