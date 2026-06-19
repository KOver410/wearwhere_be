//go:build integration

package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/order/domain"
	promodomain "github.com/wearwhere/wearwhere_be/internal/promo/domain"
	"github.com/wearwhere/wearwhere_be/internal/testfixtures"
)

// seedPromo inserts a promo code and returns its id.
func seedPromo(t *testing.T, pool *pgxpool.Pool, code string, dtype promodomain.DiscountType, value, minOrder int64, active bool, endsAt *time.Time) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := pool.QueryRow(context.Background(),
		`INSERT INTO promo_codes
		   (code, discount_type, discount_value, min_order_value_vnd, starts_at, ends_at, is_active)
		 VALUES ($1, $2, $3, $4, NOW() - INTERVAL '1 hour', $5, $6)
		 RETURNING id`,
		code, dtype, value, minOrder, endsAt, active).Scan(&id)
	require.NoError(t, err)
	return id
}

func codSelections(s testSetup) []domain.ShippingSelection {
	return []domain.ShippingSelection{{BrandID: s.BrandID, Carrier: "flat"}}
}

func TestPlaceOrder_Promo_FixedDiscount(t *testing.T) {
	s := setupOrder(t, 10, 100000) // subtotal 100000, shipping 30000
	ctx := context.Background()
	seedPromo(t, s.Pool, "GIAM20K", promodomain.DiscountTypeFixed, 20000, 0, true, nil)

	resp, _, err := s.Svc.PlaceOrder(ctx, s.UserID, domain.PlaceOrderReq{
		AddressID:          s.AddrID,
		PaymentMethod:      domain.PaymentMethodCOD,
		PromoCode:          "giam20k",
		ShippingSelections: codSelections(s),
	})
	require.NoError(t, err)
	assert.Equal(t, int64(20000), resp.DiscountVND)
	assert.Equal(t, "giam20k", resp.PromoCode)
	// grand = 100000 + 30000 - 20000
	assert.Equal(t, int64(110000), resp.GrandTotalVND)

	// Redemption recorded.
	var cnt int
	require.NoError(t, s.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM promo_redemptions WHERE user_id=$1`, s.UserID).Scan(&cnt))
	assert.Equal(t, 1, cnt)
}

func TestPlaceOrder_Promo_PercentageDiscount(t *testing.T) {
	s := setupOrder(t, 10, 100000)
	ctx := context.Background()
	seedPromo(t, s.Pool, "GIAM10PCT", promodomain.DiscountTypePercentage, 10, 0, true, nil)

	resp, _, err := s.Svc.PlaceOrder(ctx, s.UserID, domain.PlaceOrderReq{
		AddressID:          s.AddrID,
		PaymentMethod:      domain.PaymentMethodCOD,
		PromoCode:          "GIAM10PCT",
		ShippingSelections: codSelections(s),
	})
	require.NoError(t, err)
	assert.Equal(t, int64(10000), resp.DiscountVND) // 10% of 100000
	assert.Equal(t, int64(120000), resp.GrandTotalVND)
}

func TestPlaceOrder_Promo_SecondUseSameUser_409(t *testing.T) {
	s := setupOrder(t, 10, 100000)
	ctx := context.Background()
	seedPromo(t, s.Pool, "ONCEONLY", promodomain.DiscountTypeFixed, 10000, 0, true, nil)

	_, _, err := s.Svc.PlaceOrder(ctx, s.UserID, domain.PlaceOrderReq{
		AddressID: s.AddrID, PaymentMethod: domain.PaymentMethodCOD,
		PromoCode: "ONCEONLY", ShippingSelections: codSelections(s),
	})
	require.NoError(t, err)

	// Re-seed a cart item (first order cleared the cart) then re-attempt.
	reseedCart(t, s)
	_, _, err = s.Svc.PlaceOrder(ctx, s.UserID, domain.PlaceOrderReq{
		AddressID: s.AddrID, PaymentMethod: domain.PaymentMethodCOD,
		PromoCode: "ONCEONLY", ShippingSelections: codSelections(s),
	})
	assert.ErrorIs(t, err, promodomain.ErrPromoAlreadyUsed)
}

func TestPlaceOrder_Promo_Expired(t *testing.T) {
	s := setupOrder(t, 10, 100000)
	ctx := context.Background()
	past := time.Now().Add(-time.Minute)
	seedPromo(t, s.Pool, "EXPIRED", promodomain.DiscountTypeFixed, 10000, 0, true, &past)

	_, _, err := s.Svc.PlaceOrder(ctx, s.UserID, domain.PlaceOrderReq{
		AddressID: s.AddrID, PaymentMethod: domain.PaymentMethodCOD,
		PromoCode: "EXPIRED", ShippingSelections: codSelections(s),
	})
	assert.ErrorIs(t, err, promodomain.ErrPromoExpired)
}

func TestPlaceOrder_Promo_MinOrderNotMet(t *testing.T) {
	s := setupOrder(t, 10, 100000) // subtotal 100000
	ctx := context.Background()
	seedPromo(t, s.Pool, "BIGSPEND", promodomain.DiscountTypeFixed, 10000, 500000, true, nil)

	_, _, err := s.Svc.PlaceOrder(ctx, s.UserID, domain.PlaceOrderReq{
		AddressID: s.AddrID, PaymentMethod: domain.PaymentMethodCOD,
		PromoCode: "BIGSPEND", ShippingSelections: codSelections(s),
	})
	assert.ErrorIs(t, err, promodomain.ErrPromoMinOrder)
}

func TestPlaceOrder_Promo_UnknownCode(t *testing.T) {
	s := setupOrder(t, 10, 100000)
	ctx := context.Background()

	_, _, err := s.Svc.PlaceOrder(ctx, s.UserID, domain.PlaceOrderReq{
		AddressID: s.AddrID, PaymentMethod: domain.PaymentMethodCOD,
		PromoCode: "DOESNOTEXIST", ShippingSelections: codSelections(s),
	})
	assert.ErrorIs(t, err, promodomain.ErrPromoNotFound)
}

func TestPlaceOrder_NoPromo_Unaffected(t *testing.T) {
	s := setupOrder(t, 10, 100000)
	ctx := context.Background()

	resp, _, err := s.Svc.PlaceOrder(ctx, s.UserID, domain.PlaceOrderReq{
		AddressID: s.AddrID, PaymentMethod: domain.PaymentMethodCOD,
		ShippingSelections: codSelections(s),
	})
	require.NoError(t, err)
	assert.Equal(t, int64(0), resp.DiscountVND)
	assert.Empty(t, resp.PromoCode)
	assert.Equal(t, int64(130000), resp.GrandTotalVND)
}

// reseedCart re-adds the seeded variant to the cart (qty 1) for a follow-up order.
func reseedCart(t *testing.T, s testSetup) {
	t.Helper()
	testfixtures.SeedCartItem(t, s.Pool, s.UserID, s.VariantID, 1, 100000)
}
