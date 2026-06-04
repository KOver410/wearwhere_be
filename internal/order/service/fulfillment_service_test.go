//go:build integration

package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	brandrepo "github.com/wearwhere/wearwhere_be/internal/brand/repo"
	"github.com/wearwhere/wearwhere_be/internal/order/domain"
	orderrepo "github.com/wearwhere/wearwhere_be/internal/order/repo"
	"github.com/wearwhere/wearwhere_be/internal/order/service"
	"github.com/wearwhere/wearwhere_be/internal/shipping/goship"
	"github.com/wearwhere/wearwhere_be/internal/shipping/weight"
)

// buildFulfillmentSvc constructs a FulfillmentService backed by the test pool using mock goship.
func buildFulfillmentSvc(s testSetup) *service.FulfillmentService {
	return service.NewFulfillmentService(
		s.Pool,
		orderrepo.NewOrderPG(s.Pool),
		orderrepo.NewSubOrderPG(s.Pool),
		orderrepo.NewOrderItemPG(s.Pool),
		goship.NewMockClient(),
		brandrepo.NewAddressPG(s.Pool),
		weight.Defaults{WeightG: 500, LengthCM: 20, WidthCM: 15, HeightCM: 10},
	)
}

// placeCODGoshipOrder places a COD Goship order and returns the sub-order ID.
func placeCODGoshipOrder(t *testing.T, s testSetup) (orderID, subOrderID uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	resp, _, err := s.Svc.PlaceOrder(ctx, s.UserID, domain.PlaceOrderReq{
		AddressID:     s.AddrID,
		PaymentMethod: domain.PaymentMethodCOD,
		ShippingSelections: []domain.ShippingSelection{
			{BrandID: s.BrandID, Carrier: "Giao Hàng Nhanh (v3)"},
		},
	})
	require.NoError(t, err)

	// Fetch the sub-order for the placed order.
	err = s.Pool.QueryRow(ctx,
		`SELECT so.order_id, so.id
		   FROM sub_orders so
		   JOIN orders o ON o.id = so.order_id
		  WHERE o.user_id = $1
		  ORDER BY so.created_at DESC
		  LIMIT 1`,
		s.UserID).Scan(&orderID, &subOrderID)
	require.NoError(t, err)
	_ = resp
	return orderID, subOrderID
}

// TestFulfillment_ConfirmThenShip_COD tests the full confirm → ship lifecycle.
func TestFulfillment_ConfirmThenShip_COD(t *testing.T) {
	s := setupGoshipOrder(t, 1, 100000)
	ctx := context.Background()
	fulfillSvc := buildFulfillmentSvc(s)

	_, subOrderID := placeCODGoshipOrder(t, s)

	// --- Confirm ---
	err := fulfillSvc.Confirm(ctx, s.BrandID, subOrderID)
	require.NoError(t, err)

	// Assert DB state after confirm.
	var status string
	var confirmedAt *string
	err = s.Pool.QueryRow(ctx,
		`SELECT status, confirmed_at::text FROM sub_orders WHERE id=$1`, subOrderID,
	).Scan(&status, &confirmedAt)
	require.NoError(t, err)
	require.Equal(t, "confirmed", status)
	require.NotNil(t, confirmedAt, "confirmed_at must be set after Confirm")

	// Capture shipping_fee_vnd before Ship (must remain unchanged after Ship).
	var shippingFeeVND int64
	err = s.Pool.QueryRow(ctx,
		`SELECT shipping_fee_vnd FROM sub_orders WHERE id=$1`, subOrderID,
	).Scan(&shippingFeeVND)
	require.NoError(t, err)

	// --- Ship ---
	err = fulfillSvc.Ship(ctx, s.BrandID, subOrderID, "")
	require.NoError(t, err)

	// Assert DB state after ship.
	var trackingNo, goshipCode string
	var shippingCostVND int64
	var shippedAt *string
	var shippingFeeAfter int64
	err = s.Pool.QueryRow(ctx,
		`SELECT status, tracking_no, goship_shipment_code, shipping_cost_vnd, shipped_at::text, shipping_fee_vnd
		   FROM sub_orders WHERE id=$1`, subOrderID,
	).Scan(&status, &trackingNo, &goshipCode, &shippingCostVND, &shippedAt, &shippingFeeAfter)
	require.NoError(t, err)

	require.Equal(t, "shipped", status)
	require.Regexp(t, `^MOCK-TRK-\d+$`, trackingNo)
	require.Regexp(t, `^MOCK-GS-\d+$`, goshipCode)
	require.Equal(t, int64(20000), shippingCostVND)
	require.NotNil(t, shippedAt, "shipped_at must be set after Ship")
	// shipping_fee_vnd (customer-visible placement fee) must NOT change.
	require.Equal(t, shippingFeeVND, shippingFeeAfter, "shipping_fee_vnd must not change after Ship")
}

// TestFulfillment_CrossBrandForbidden ensures Confirm returns ErrNotBrandOwner for a wrong brand.
func TestFulfillment_CrossBrandForbidden(t *testing.T) {
	s := setupGoshipOrder(t, 1, 100000)
	ctx := context.Background()
	fulfillSvc := buildFulfillmentSvc(s)

	_, subOrderID := placeCODGoshipOrder(t, s)

	otherBrandID := uuid.New()
	err := fulfillSvc.Confirm(ctx, otherBrandID, subOrderID)
	require.ErrorIs(t, err, domain.ErrNotBrandOwner)
}

// TestFulfillment_ShipBeforeConfirm_Invalid ensures Ship on a pending sub-order returns ErrInvalidTransition.
func TestFulfillment_ShipBeforeConfirm_Invalid(t *testing.T) {
	s := setupGoshipOrder(t, 1, 100000)
	ctx := context.Background()
	fulfillSvc := buildFulfillmentSvc(s)

	_, subOrderID := placeCODGoshipOrder(t, s)

	// Attempt to ship without confirming first.
	err := fulfillSvc.Ship(ctx, s.BrandID, subOrderID, "")
	require.ErrorIs(t, err, domain.ErrInvalidTransition)
}

// TestFulfillment_List verifies that List returns at least one item with correct fields.
func TestFulfillment_List(t *testing.T) {
	s := setupGoshipOrder(t, 1, 100000)
	ctx := context.Background()
	fulfillSvc := buildFulfillmentSvc(s)

	_, _ = placeCODGoshipOrder(t, s)

	resp, err := fulfillSvc.List(ctx, s.BrandID, nil, 1, 20)
	require.NoError(t, err)
	require.GreaterOrEqual(t, resp.Total, 1)
	require.NotEmpty(t, resp.Data)

	item := resp.Data[0]
	require.NotEqual(t, uuid.Nil, item.SubOrderID)
	require.NotEmpty(t, item.OrderNo, "OrderNo must be set")
	require.NotEmpty(t, item.Recipient, "Recipient must be set")
	require.GreaterOrEqual(t, item.ItemCount, 1)

	// Verify the listed item belongs to the correct brand.
	var listedBrandID uuid.UUID
	err = s.Pool.QueryRow(ctx,
		`SELECT brand_id FROM sub_orders WHERE id=$1`, item.SubOrderID,
	).Scan(&listedBrandID)
	require.NoError(t, err)
	require.Equal(t, s.BrandID, listedBrandID)
}

// TestFulfillment_List_FilterByStatus verifies that filtering by status works correctly.
func TestFulfillment_List_FilterByStatus(t *testing.T) {
	s := setupGoshipOrder(t, 1, 100000)
	ctx := context.Background()
	fulfillSvc := buildFulfillmentSvc(s)

	_, subOrderID := placeCODGoshipOrder(t, s)

	// Before confirming, filter by "pending" — should return the item.
	resp, err := fulfillSvc.List(ctx, s.BrandID, []domain.SubOrderStatus{domain.SubOrderStatusPending}, 1, 20)
	require.NoError(t, err)
	require.GreaterOrEqual(t, resp.Total, 1)

	found := false
	for _, item := range resp.Data {
		if item.SubOrderID == subOrderID {
			found = true
			require.Equal(t, domain.SubOrderStatusPending, item.Status)
		}
	}
	require.True(t, found, "seeded sub-order must appear in pending filter")

	// Filter by "confirmed" — should NOT include the pending sub-order.
	respConf, err := fulfillSvc.List(ctx, s.BrandID, []domain.SubOrderStatus{domain.SubOrderStatusConfirmed}, 1, 20)
	require.NoError(t, err)
	for _, item := range respConf.Data {
		require.NotEqual(t, subOrderID, item.SubOrderID)
	}
}
