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
	paymentrepo "github.com/wearwhere/wearwhere_be/internal/payment/repo"
	productrepo "github.com/wearwhere/wearwhere_be/internal/product/repo"
	"github.com/wearwhere/wearwhere_be/internal/shipping/goship"
	"github.com/wearwhere/wearwhere_be/internal/shipping/weight"
	"github.com/wearwhere/wearwhere_be/internal/testfixtures"
)

// buildWebhookSvc constructs a ShippingWebhookService backed by the test pool.
func buildWebhookSvc(s testSetup) *service.ShippingWebhookService {
	return service.NewShippingWebhookService(
		s.Pool,
		orderrepo.NewSubOrderPG(s.Pool),
		orderrepo.NewOrderPG(s.Pool),
		orderrepo.NewOrderItemPG(s.Pool),
		paymentrepo.NewPaymentPG(s.Pool),
		productrepo.NewVariantPG(s.Pool),
	)
}

// setupGoshipOrderClean wipes the DB then seeds a fresh scenario so tracking-number
// collisions cannot occur across tests (the mock client seq always resets to 0).
func setupGoshipOrderClean(t *testing.T, qty int, price float64) testSetup {
	t.Helper()
	cleanPool := testfixtures.MustPool(t)
	testfixtures.Clean(t, cleanPool)
	return setupGoshipOrder(t, qty, price)
}

// placeConfirmShipCOD places a COD Goship order, confirms it, and ships it using
// the provided mock client. Returns the orderID, subOrderID, and trackingNo.
func placeConfirmShipCOD(t *testing.T, s testSetup, mockClient *goship.MockClient) (orderID, subOrderID uuid.UUID, trackingNo string) {
	t.Helper()
	ctx := context.Background()

	_, _, err := s.Svc.PlaceOrder(ctx, s.UserID, domain.PlaceOrderReq{
		AddressID:     s.AddrID,
		PaymentMethod: domain.PaymentMethodCOD,
		ShippingSelections: []domain.ShippingSelection{
			{BrandID: s.BrandID, Carrier: "Giao Hàng Nhanh (v3)"},
		},
	})
	require.NoError(t, err)

	err = s.Pool.QueryRow(ctx,
		`SELECT o.id, so.id
		   FROM sub_orders so
		   JOIN orders o ON o.id = so.order_id
		  WHERE o.user_id = $1
		  ORDER BY so.created_at DESC
		  LIMIT 1`,
		s.UserID).Scan(&orderID, &subOrderID)
	require.NoError(t, err)

	fulfillSvc := service.NewFulfillmentService(
		orderrepo.NewOrderPG(s.Pool),
		orderrepo.NewSubOrderPG(s.Pool),
		orderrepo.NewOrderItemPG(s.Pool),
		mockClient,
		brandrepo.NewAddressPG(s.Pool),
		weight.Defaults{WeightG: 500, LengthCM: 20, WidthCM: 15, HeightCM: 10},
	)

	err = fulfillSvc.Confirm(ctx, s.BrandID, subOrderID)
	require.NoError(t, err)

	err = fulfillSvc.Ship(ctx, s.BrandID, subOrderID, "")
	require.NoError(t, err)

	err = s.Pool.QueryRow(ctx,
		`SELECT tracking_no FROM sub_orders WHERE id=$1`, subOrderID,
	).Scan(&trackingNo)
	require.NoError(t, err)
	require.NotEmpty(t, trackingNo)

	return orderID, subOrderID, trackingNo
}

// TestWebhook_Delivered_COD_CompletesAndSettles tests the full COD delivery webhook flow:
// delivered webhook → sub_order delivered, order completed, payment paid, stock committed.
func TestWebhook_Delivered_COD_CompletesAndSettles(t *testing.T) {
	s := setupGoshipOrderClean(t, 1, 100000)
	ctx := context.Background()
	mockClient := goship.NewMockClient()

	// Capture stock before placement.
	stockBefore, _ := testfixtures.GetVariantStock(t, s.Pool, s.VariantID)

	orderID, subOrderID, trackingNo := placeConfirmShipCOD(t, s, mockClient)

	// After placement, reserved_qty should equal the ordered qty (1).
	_, reservedAfterPlace := testfixtures.GetVariantStock(t, s.Pool, s.VariantID)
	require.Equal(t, 1, reservedAfterPlace, "reserved_qty must be 1 after placement of qty=1")

	// Fire the delivered webhook.
	webhookSvc := buildWebhookSvc(s)
	err := webhookSvc.HandleGoshipWebhook(ctx, goship.WebhookPayload{
		Code:       trackingNo,
		StatusText: "Đã giao hàng",
	})
	require.NoError(t, err)

	// Assert sub_orders.status = 'delivered', delivered_at not null.
	var subStatus string
	var deliveredAt *string
	err = s.Pool.QueryRow(ctx,
		`SELECT status, delivered_at::text FROM sub_orders WHERE id=$1`, subOrderID,
	).Scan(&subStatus, &deliveredAt)
	require.NoError(t, err)
	require.Equal(t, "delivered", subStatus)
	require.NotNil(t, deliveredAt, "delivered_at must be set after delivery webhook")

	// Assert orders.status = 'completed' (single-brand → all sub-orders delivered).
	var orderStatus string
	err = s.Pool.QueryRow(ctx,
		`SELECT status FROM orders WHERE id=$1`, orderID,
	).Scan(&orderStatus)
	require.NoError(t, err)
	require.Equal(t, "completed", orderStatus)

	// Assert payment status = 'paid'.
	var payStatus string
	err = s.Pool.QueryRow(ctx,
		`SELECT status FROM payments WHERE order_id=$1`, orderID,
	).Scan(&payStatus)
	require.NoError(t, err)
	require.Equal(t, "paid", payStatus)

	// Assert stock committed: reserved_qty = 0, stock_qty = stockBefore-1.
	stockAfter, reservedAfter := testfixtures.GetVariantStock(t, s.Pool, s.VariantID)
	require.Equal(t, 0, reservedAfter, "reserved_qty must be 0 after COD delivery commit")
	require.Equal(t, stockBefore-1, stockAfter, "stock_qty must decrease by ordered qty on COD delivery")
}

// TestWebhook_Idempotent fires the same delivered webhook twice and asserts no double-commit.
func TestWebhook_Idempotent(t *testing.T) {
	s := setupGoshipOrderClean(t, 1, 100000)
	ctx := context.Background()
	mockClient := goship.NewMockClient()

	orderID, _, trackingNo := placeConfirmShipCOD(t, s, mockClient)

	webhookSvc := buildWebhookSvc(s)
	payload := goship.WebhookPayload{Code: trackingNo, StatusText: "Đã giao hàng"}

	// First delivery webhook.
	err := webhookSvc.HandleGoshipWebhook(ctx, payload)
	require.NoError(t, err)

	stockAfterFirst, reservedAfterFirst := testfixtures.GetVariantStock(t, s.Pool, s.VariantID)

	// Second delivery webhook (idempotent — must be a no-op).
	err = webhookSvc.HandleGoshipWebhook(ctx, payload)
	require.NoError(t, err)

	stockAfterSecond, reservedAfterSecond := testfixtures.GetVariantStock(t, s.Pool, s.VariantID)
	require.Equal(t, stockAfterFirst, stockAfterSecond, "stock_qty must not change on duplicate webhook")
	require.Equal(t, reservedAfterFirst, reservedAfterSecond, "reserved_qty must not change on duplicate webhook")

	// Order still completed, payment still paid.
	var orderStatus, payStatus string
	err = s.Pool.QueryRow(ctx, `SELECT status FROM orders WHERE id=$1`, orderID).Scan(&orderStatus)
	require.NoError(t, err)
	require.Equal(t, "completed", orderStatus)

	err = s.Pool.QueryRow(ctx, `SELECT status FROM payments WHERE order_id=$1`, orderID).Scan(&payStatus)
	require.NoError(t, err)
	require.Equal(t, "paid", payStatus)
}

// TestWebhook_UnknownTracking_NoOp verifies that an unknown tracking code is silently ignored.
func TestWebhook_UnknownTracking_NoOp(t *testing.T) {
	s := setupGoshipOrder(t, 1, 100000)
	ctx := context.Background()

	webhookSvc := buildWebhookSvc(s)
	err := webhookSvc.HandleGoshipWebhook(ctx, goship.WebhookPayload{
		Code:       "DOES-NOT-EXIST",
		StatusText: "Đã giao hàng",
	})
	require.NoError(t, err, "unknown tracking code must be silently tolerated")
}

// TestWebhook_Delivered_Payos_NoStockChangeOnDelivery tests PayOS order delivery.
// PayOS commits stock at payment time, so delivery must NOT commit stock again.
func TestWebhook_Delivered_Payos_NoStockChangeOnDelivery(t *testing.T) {
	t.Skip("PayOS paid flow requires simulating a full PayOS payment webhook to reach " +
		"the 'processing' state with stock already committed. " +
		"A SimulatePayosPaid test fixture is not yet available in the harness. " +
		"The COD delivery path is fully covered by TestWebhook_Delivered_COD_CompletesAndSettles.")
}
