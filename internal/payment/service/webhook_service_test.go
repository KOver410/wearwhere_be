//go:build integration

package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	authrepo "github.com/wearwhere/wearwhere_be/internal/auth/repo"
	brandrepo "github.com/wearwhere/wearwhere_be/internal/brand/repo"
	customeraddrrepo "github.com/wearwhere/wearwhere_be/internal/customeraddr/repo"
	orderdomain "github.com/wearwhere/wearwhere_be/internal/order/domain"
	orderrepo "github.com/wearwhere/wearwhere_be/internal/order/repo"
	orderservice "github.com/wearwhere/wearwhere_be/internal/order/service"
	"github.com/wearwhere/wearwhere_be/internal/payment/payos"
	paymentrepo "github.com/wearwhere/wearwhere_be/internal/payment/repo"
	"github.com/wearwhere/wearwhere_be/internal/payment/service"
	productrepo "github.com/wearwhere/wearwhere_be/internal/product/repo"
	"github.com/wearwhere/wearwhere_be/internal/shipping/provider"
	"github.com/wearwhere/wearwhere_be/internal/testfixtures"
)

// webhookSetup seeds a full order+payment and returns the payos_order_code to use in
// webhook payloads, along with the variant ID and all repos needed for assertions.
type webhookSetup struct {
	OrderCode int64
	VariantID interface{ String() string }
	Services  *service.WebhookService
	Pool      interface {
		QueryRow(ctx context.Context, sql string, args ...interface{}) interface {
			Scan(dest ...interface{}) error
		}
	}
}

func buildWebhookSvc(t *testing.T) *service.WebhookService {
	t.Helper()
	pool := testfixtures.MustPool(t)
	return service.NewWebhookService(
		pool,
		paymentrepo.NewPaymentPG(pool),
		orderrepo.NewOrderPG(pool),
		orderrepo.NewSubOrderPG(pool),
		orderrepo.NewOrderItemPG(pool),
		productrepo.NewVariantPG(pool),
		payos.NewMockClient(""),
	)
}

func placePayosOrder(t *testing.T) (orderCode int64, variantID interface{}) {
	t.Helper()
	pool := testfixtures.MustPool(t)
	ctx := context.Background()

	customer := testfixtures.SeedCustomer(t, pool)
	addr := testfixtures.SeedCustomerAddress(t, pool, customer.ID, testfixtures.CustomerAddressOpts{IsDefault: true})
	brand := testfixtures.SeedBrand(t, pool, [16]byte{})
	cat := testfixtures.SeedCategory(t, pool)
	prod := testfixtures.SeedProduct(t, pool, brand.ID, cat.ID, "active")
	vID := testfixtures.SeedVariant(t, pool, prod.ID, "M", "Black", 100000, 5)

	_, _ = pool.Exec(ctx, `UPDATE brands SET shipping_flat_fee_vnd = 30000 WHERE id = $1`, brand.ID)
	testfixtures.SeedCartItem(t, pool, customer.ID, vID, 1, 100000)

	svc := orderservice.NewOrderService(
		pool,
		orderrepo.NewOrderPG(pool),
		orderrepo.NewSubOrderPG(pool),
		orderrepo.NewOrderItemPG(pool),
		paymentrepo.NewPaymentPG(pool),
		productrepo.NewVariantPG(pool),
		customeraddrrepo.NewAddressPG(pool),
		authrepo.NewUserPG(pool),
		provider.NewFlatRateProvider(brandrepo.NewBrandPG(pool)),
		payos.NewMockClient(""),
		orderservice.Config{
			ReservationTimeout: 30 * time.Minute,
			PayosReturnURL:     "http://ret",
			PayosCancelURL:     "http://can",
		},
	)

	_, pay, err := svc.PlaceOrder(ctx, customer.ID, orderdomain.PlaceOrderReq{
		AddressID:          addr.ID,
		PaymentMethod:      orderdomain.PaymentMethodPayos,
		ShippingSelections: []orderdomain.ShippingSelection{{BrandID: brand.ID, Carrier: "flat"}},
	})
	require.NoError(t, err)
	require.NotNil(t, pay)

	// Retrieve the payos_order_code from the payments table.
	var code int64
	err = pool.QueryRow(ctx,
		`SELECT payos_order_code FROM payments WHERE order_id = (
			SELECT id FROM orders WHERE user_id = $1 ORDER BY created_at DESC LIMIT 1
		)`, customer.ID).Scan(&code)
	require.NoError(t, err)

	return code, vID
}

func TestWebhook_Success_CommitsStockAndOrder(t *testing.T) {
	pool := testfixtures.MustPool(t)
	ctx := context.Background()

	customer := testfixtures.SeedCustomer(t, pool)
	addr := testfixtures.SeedCustomerAddress(t, pool, customer.ID, testfixtures.CustomerAddressOpts{IsDefault: true})
	brand := testfixtures.SeedBrand(t, pool, [16]byte{})
	cat := testfixtures.SeedCategory(t, pool)
	prod := testfixtures.SeedProduct(t, pool, brand.ID, cat.ID, "active")
	variantID := testfixtures.SeedVariant(t, pool, prod.ID, "M", "Black", 100000, 5)

	_, _ = pool.Exec(ctx, `UPDATE brands SET shipping_flat_fee_vnd = 30000 WHERE id = $1`, brand.ID)
	testfixtures.SeedCartItem(t, pool, customer.ID, variantID, 1, 100000)

	orderSvc := orderservice.NewOrderService(
		pool,
		orderrepo.NewOrderPG(pool),
		orderrepo.NewSubOrderPG(pool),
		orderrepo.NewOrderItemPG(pool),
		paymentrepo.NewPaymentPG(pool),
		productrepo.NewVariantPG(pool),
		customeraddrrepo.NewAddressPG(pool),
		authrepo.NewUserPG(pool),
		provider.NewFlatRateProvider(brandrepo.NewBrandPG(pool)),
		payos.NewMockClient(""),
		orderservice.Config{
			ReservationTimeout: 30 * time.Minute,
			PayosReturnURL:     "http://ret",
			PayosCancelURL:     "http://can",
		},
	)

	_, _, err := orderSvc.PlaceOrder(ctx, customer.ID, orderdomain.PlaceOrderReq{
		AddressID:          addr.ID,
		PaymentMethod:      orderdomain.PaymentMethodPayos,
		ShippingSelections: []orderdomain.ShippingSelection{{BrandID: brand.ID, Carrier: "flat"}},
	})
	require.NoError(t, err)

	// Fetch the payos_order_code from payments table.
	var orderCode int64
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT payos_order_code FROM payments
		   WHERE order_id = (SELECT id FROM orders WHERE user_id = $1 ORDER BY created_at DESC LIMIT 1)`,
		customer.ID).Scan(&orderCode))

	// Fire success webhook.
	webhookSvc := service.NewWebhookService(
		pool,
		paymentrepo.NewPaymentPG(pool),
		orderrepo.NewOrderPG(pool),
		orderrepo.NewSubOrderPG(pool),
		orderrepo.NewOrderItemPG(pool),
		productrepo.NewVariantPG(pool),
		payos.NewMockClient(""),
	)

	err = webhookSvc.HandlePayosWebhook(ctx, payos.WebhookPayload{
		Code:    "00",
		Desc:    "success",
		Success: true,
		Data:    payos.WebhookData{OrderCode: orderCode, Code: "00", Desc: "success"},
	})
	require.NoError(t, err)

	// Assert order status = 'processing', payment_status = 'paid'.
	var orderStatus, paymentStatus string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT status, payment_status FROM orders WHERE user_id = $1 ORDER BY created_at DESC LIMIT 1`,
		customer.ID).Scan(&orderStatus, &paymentStatus))
	require.Equal(t, "processing", orderStatus)
	require.Equal(t, "paid", paymentStatus)

	// Assert payment status = 'paid'.
	var pmtStatus string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT status FROM payments WHERE payos_order_code = $1`, orderCode).Scan(&pmtStatus))
	require.Equal(t, "paid", pmtStatus)

	// Assert stock_qty decremented by 1, reserved_qty = 0.
	stock, reserved := testfixtures.GetVariantStock(t, pool, variantID)
	require.Equal(t, 4, stock)    // 5 - 1 committed
	require.Equal(t, 0, reserved) // reservation cleared
}

func TestWebhook_Idempotent_SecondCallNoOp(t *testing.T) {
	pool := testfixtures.MustPool(t)
	ctx := context.Background()

	customer := testfixtures.SeedCustomer(t, pool)
	addr := testfixtures.SeedCustomerAddress(t, pool, customer.ID, testfixtures.CustomerAddressOpts{IsDefault: true})
	brand := testfixtures.SeedBrand(t, pool, [16]byte{})
	cat := testfixtures.SeedCategory(t, pool)
	prod := testfixtures.SeedProduct(t, pool, brand.ID, cat.ID, "active")
	variantID := testfixtures.SeedVariant(t, pool, prod.ID, "M", "Black", 100000, 5)

	_, _ = pool.Exec(ctx, `UPDATE brands SET shipping_flat_fee_vnd = 30000 WHERE id = $1`, brand.ID)
	testfixtures.SeedCartItem(t, pool, customer.ID, variantID, 1, 100000)

	orderSvc := orderservice.NewOrderService(
		pool,
		orderrepo.NewOrderPG(pool),
		orderrepo.NewSubOrderPG(pool),
		orderrepo.NewOrderItemPG(pool),
		paymentrepo.NewPaymentPG(pool),
		productrepo.NewVariantPG(pool),
		customeraddrrepo.NewAddressPG(pool),
		authrepo.NewUserPG(pool),
		provider.NewFlatRateProvider(brandrepo.NewBrandPG(pool)),
		payos.NewMockClient(""),
		orderservice.Config{
			ReservationTimeout: 30 * time.Minute,
			PayosReturnURL:     "http://ret",
			PayosCancelURL:     "http://can",
		},
	)

	_, _, err := orderSvc.PlaceOrder(ctx, customer.ID, orderdomain.PlaceOrderReq{
		AddressID:          addr.ID,
		PaymentMethod:      orderdomain.PaymentMethodPayos,
		ShippingSelections: []orderdomain.ShippingSelection{{BrandID: brand.ID, Carrier: "flat"}},
	})
	require.NoError(t, err)

	var orderCode int64
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT payos_order_code FROM payments
		   WHERE order_id = (SELECT id FROM orders WHERE user_id = $1 ORDER BY created_at DESC LIMIT 1)`,
		customer.ID).Scan(&orderCode))

	webhookSvc := service.NewWebhookService(
		pool,
		paymentrepo.NewPaymentPG(pool),
		orderrepo.NewOrderPG(pool),
		orderrepo.NewSubOrderPG(pool),
		orderrepo.NewOrderItemPG(pool),
		productrepo.NewVariantPG(pool),
		payos.NewMockClient(""),
	)

	payload := payos.WebhookPayload{
		Code:    "00",
		Desc:    "success",
		Success: true,
		Data:    payos.WebhookData{OrderCode: orderCode, Code: "00", Desc: "success"},
	}

	// First call — should succeed.
	require.NoError(t, webhookSvc.HandlePayosWebhook(ctx, payload))

	// Second call — should be a no-op (idempotent), not return an error.
	require.NoError(t, webhookSvc.HandlePayosWebhook(ctx, payload))

	// Stock should only be committed once: stock_qty=4, reserved_qty=0.
	stock, reserved := testfixtures.GetVariantStock(t, pool, variantID)
	require.Equal(t, 4, stock)
	require.Equal(t, 0, reserved)
}
