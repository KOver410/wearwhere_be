//go:build integration

package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/order/domain"
	orderrepo "github.com/wearwhere/wearwhere_be/internal/order/repo"
	"github.com/wearwhere/wearwhere_be/internal/testfixtures"
)

// TestCancel_CODPending_ReleasesStock places a COD order (which sets reserved_qty=1),
// then cancels it and verifies reserved_qty returns to 0 and status is cancelled.
func TestCancel_CODPending_ReleasesStock(t *testing.T) {
	s := setupOrder(t, 5, 100000)
	ctx := context.Background()

	// Place COD order.
	resp, _, err := s.Svc.PlaceOrder(ctx, s.UserID, domain.PlaceOrderReq{
		AddressID:     s.AddrID,
		PaymentMethod: domain.PaymentMethodCOD,
		ShippingSelections: []domain.ShippingSelection{
			{BrandID: s.BrandID, Carrier: "flat"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, domain.OrderStatusProcessing, resp.Status)

	// Stock should be reserved after placement.
	_, reserved := testfixtures.GetVariantStock(t, s.Pool, s.VariantID)
	require.Equal(t, 1, reserved, "reserved_qty should be 1 after placement")

	// Cancel the order.
	cancelResp, err := s.Svc.Cancel(ctx, s.UserID, resp.OrderNo, "changed my mind")
	require.NoError(t, err)
	require.Equal(t, domain.OrderStatusCancelled, cancelResp.Status)
	require.Equal(t, domain.PaymentStatusCancelled, cancelResp.PaymentStatus)

	// Reserved qty should be released.
	_, reservedAfter := testfixtures.GetVariantStock(t, s.Pool, s.VariantID)
	require.Equal(t, 0, reservedAfter, "reserved_qty should be 0 after cancel")
}

// TestCancel_OtherUser_NotFound ensures that a user cannot cancel another user's order.
func TestCancel_OtherUser_NotFound(t *testing.T) {
	s := setupOrder(t, 5, 100000)
	ctx := context.Background()

	// Place as user A.
	resp, _, err := s.Svc.PlaceOrder(ctx, s.UserID, domain.PlaceOrderReq{
		AddressID:     s.AddrID,
		PaymentMethod: domain.PaymentMethodCOD,
		ShippingSelections: []domain.ShippingSelection{
			{BrandID: s.BrandID, Carrier: "flat"},
		},
	})
	require.NoError(t, err)

	// Attempt cancel as user B (random UUID).
	otherUserID := uuid.New()
	_, err = s.Svc.Cancel(ctx, otherUserID, resp.OrderNo, "")
	require.ErrorIs(t, err, domain.ErrOrderNotFound)
}

// TestList_ReturnsPagedOrders places 3 COD orders and verifies List returns Total=3 with 3 items.
func TestList_ReturnsPagedOrders(t *testing.T) {
	s := setupOrder(t, 10, 100000)
	ctx := context.Background()

	// Place 3 orders; re-add cart item before each placement since PlaceOrder clears the cart.
	// Use ON CONFLICT DO UPDATE because there is a unique constraint on (user_id, variant_id).
	for i := 0; i < 3; i++ {
		_, err := s.Pool.Exec(ctx,
			`INSERT INTO cart_items (user_id, variant_id, qty, price_snapshot, currency_snapshot)
			 VALUES ($1, $2, 1, 100000, 'VND')
			 ON CONFLICT (user_id, variant_id) DO UPDATE SET qty = 1`,
			s.UserID, s.VariantID)
		require.NoError(t, err)
		_, _, err = s.Svc.PlaceOrder(ctx, s.UserID, domain.PlaceOrderReq{
			AddressID:     s.AddrID,
			PaymentMethod: domain.PaymentMethodCOD,
			ShippingSelections: []domain.ShippingSelection{
				{BrandID: s.BrandID, Carrier: "flat"},
			},
		})
		require.NoError(t, err)
	}

	listResp, err := s.Svc.List(ctx, orderrepo.ListFilter{
		UserID:   s.UserID,
		Page:     1,
		PageSize: 20,
	})
	require.NoError(t, err)
	require.Equal(t, 3, listResp.Total)
	require.Len(t, listResp.Data, 3)
	require.Equal(t, 1, listResp.TotalPages)
	// Each item should have ItemCount >= 1 and a valid order ID.
	for _, item := range listResp.Data {
		require.GreaterOrEqual(t, item.ItemCount, 1)
		require.NotEqual(t, uuid.Nil, item.ID)
	}
}
