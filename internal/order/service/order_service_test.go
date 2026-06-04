//go:build integration

package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	authrepo "github.com/wearwhere/wearwhere_be/internal/auth/repo"
	brandrepo "github.com/wearwhere/wearwhere_be/internal/brand/repo"
	customeraddrrepo "github.com/wearwhere/wearwhere_be/internal/customeraddr/repo"
	"github.com/wearwhere/wearwhere_be/internal/order/domain"
	orderrepo "github.com/wearwhere/wearwhere_be/internal/order/repo"
	"github.com/wearwhere/wearwhere_be/internal/order/service"
	"github.com/wearwhere/wearwhere_be/internal/payment/payos"
	paymentrepo "github.com/wearwhere/wearwhere_be/internal/payment/repo"
	productrepo "github.com/wearwhere/wearwhere_be/internal/product/repo"
	"github.com/wearwhere/wearwhere_be/internal/shipping/goship"
	"github.com/wearwhere/wearwhere_be/internal/shipping/provider"
	"github.com/wearwhere/wearwhere_be/internal/shipping/weight"
	"github.com/wearwhere/wearwhere_be/internal/testfixtures"
)

// testSetup holds the seeded IDs and a ready-to-use OrderService.
type testSetup struct {
	UserID    uuid.UUID
	AddrID    uuid.UUID
	BrandID   uuid.UUID
	ProductID uuid.UUID
	VariantID uuid.UUID
	Svc       *service.OrderService
	Pool      *pgxpool.Pool
}

// buildSvc constructs an OrderService backed by pool, using mock PayOS.
func buildSvc(pool *pgxpool.Pool) *service.OrderService {
	return service.NewOrderService(
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
		service.Config{
			ReservationTimeout: 30 * time.Minute,
			PayosReturnURL:     "http://ret",
			PayosCancelURL:     "http://can",
		},
	)
}

// setupOrder seeds the minimum required rows and returns the testSetup.
// stock is the initial stock_qty for the seeded variant.
// price is the price_snapshot (must be ≥ MinOrderValueVND for most tests).
func setupOrder(t *testing.T, stock int, price float64) testSetup {
	t.Helper()
	pool := testfixtures.MustPool(t)
	ctx := context.Background()

	// Seed in insertion order to satisfy FK constraints.
	customer := testfixtures.SeedCustomer(t, pool)
	addr := testfixtures.SeedCustomerAddress(t, pool, customer.ID, testfixtures.CustomerAddressOpts{IsDefault: true})
	brand := testfixtures.SeedBrand(t, pool, uuid.Nil)
	cat := testfixtures.SeedCategory(t, pool)
	prod := testfixtures.SeedProduct(t, pool, brand.ID, cat.ID, "active")
	variantID := testfixtures.SeedVariant(t, pool, prod.ID, "M", "Black", price, stock)

	// Ensure the brand has a flat shipping fee (0 is fine for total checks).
	_, _ = pool.Exec(ctx,
		`UPDATE brands SET shipping_flat_fee_vnd = 30000 WHERE id = $1`, brand.ID)

	// Seed a cart item for the customer.
	testfixtures.SeedCartItem(t, pool, customer.ID, variantID, 1, price)

	return testSetup{
		UserID:    customer.ID,
		AddrID:    addr.ID,
		BrandID:   brand.ID,
		ProductID: prod.ID,
		VariantID: variantID,
		Svc:       buildSvc(pool),
		Pool:      pool,
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestPlaceOrder_COD_Success(t *testing.T) {
	s := setupOrder(t, 10, 100000)
	ctx := context.Background()

	resp, pay, err := s.Svc.PlaceOrder(ctx, s.UserID, domain.PlaceOrderReq{
		AddressID:     s.AddrID,
		PaymentMethod: domain.PaymentMethodCOD,
		Notes:         "fast",
		ShippingSelections: []domain.ShippingSelection{
			{BrandID: s.BrandID, Carrier: "flat"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, domain.OrderStatusProcessing, resp.Status)
	require.Equal(t, domain.PaymentStatusPending, resp.PaymentStatus)
	require.Equal(t, domain.PaymentMethodCOD, pay.Method)
	require.Nil(t, pay.CheckoutURL)
}

func TestPlaceOrder_Payos_ReturnsCheckoutURL(t *testing.T) {
	s := setupOrder(t, 10, 100000)
	ctx := context.Background()

	resp, pay, err := s.Svc.PlaceOrder(ctx, s.UserID, domain.PlaceOrderReq{
		AddressID:     s.AddrID,
		PaymentMethod: domain.PaymentMethodPayos,
		ShippingSelections: []domain.ShippingSelection{
			{BrandID: s.BrandID, Carrier: "flat"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, domain.OrderStatusPendingPayment, resp.Status)
	require.NotNil(t, pay.CheckoutURL)
	require.Contains(t, *pay.CheckoutURL, "/dev/payos/mock-checkout?orderCode=")
}

func TestPlaceOrder_EmptyCart(t *testing.T) {
	s := setupOrder(t, 10, 100000)
	ctx := context.Background()

	// Clear the cart we set up.
	_, err := s.Pool.Exec(ctx, `DELETE FROM cart_items WHERE user_id=$1`, s.UserID)
	require.NoError(t, err)

	_, _, err = s.Svc.PlaceOrder(ctx, s.UserID, domain.PlaceOrderReq{
		AddressID:     s.AddrID,
		PaymentMethod: domain.PaymentMethodCOD,
	})
	require.ErrorIs(t, err, domain.ErrCartEmpty)
}

func TestPlaceOrder_MinOrderValue(t *testing.T) {
	// Price 10000 × qty 1 = 10000, below MinOrderValueVND (50000).
	s := setupOrder(t, 10, 10000)
	ctx := context.Background()

	_, _, err := s.Svc.PlaceOrder(ctx, s.UserID, domain.PlaceOrderReq{
		AddressID:     s.AddrID,
		PaymentMethod: domain.PaymentMethodCOD,
		ShippingSelections: []domain.ShippingSelection{
			{BrandID: s.BrandID, Carrier: "flat"},
		},
	})
	require.ErrorIs(t, err, domain.ErrMinOrderValue)
}

func TestPlaceOrder_AddressNotOwned(t *testing.T) {
	s := setupOrder(t, 10, 100000)
	ctx := context.Background()

	// Another user's address ID (just a random UUID that doesn't exist for the caller).
	otherAddrID := uuid.New()

	_, _, err := s.Svc.PlaceOrder(ctx, s.UserID, domain.PlaceOrderReq{
		AddressID:     otherAddrID,
		PaymentMethod: domain.PaymentMethodCOD,
	})
	require.ErrorIs(t, err, domain.ErrAddressNotFound)
}

func TestPlaceOrder_StockReservedAfterSuccess(t *testing.T) {
	s := setupOrder(t, 5, 100000)
	ctx := context.Background()

	_, _, err := s.Svc.PlaceOrder(ctx, s.UserID, domain.PlaceOrderReq{
		AddressID:     s.AddrID,
		PaymentMethod: domain.PaymentMethodPayos,
		ShippingSelections: []domain.ShippingSelection{
			{BrandID: s.BrandID, Carrier: "flat"},
		},
	})
	require.NoError(t, err)

	stock, reserved := testfixtures.GetVariantStock(t, s.Pool, s.VariantID)
	require.Equal(t, 5, stock)    // stock_qty unchanged — not committed yet
	require.Equal(t, 1, reserved) // reserved_qty incremented by 1
}

func TestPlaceOrder_ClearsCartOnSuccess(t *testing.T) {
	s := setupOrder(t, 10, 100000)
	ctx := context.Background()

	_, _, err := s.Svc.PlaceOrder(ctx, s.UserID, domain.PlaceOrderReq{
		AddressID:     s.AddrID,
		PaymentMethod: domain.PaymentMethodCOD,
		ShippingSelections: []domain.ShippingSelection{
			{BrandID: s.BrandID, Carrier: "flat"},
		},
	})
	require.NoError(t, err)

	var cnt int
	err = s.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM cart_items WHERE user_id=$1`, s.UserID).Scan(&cnt)
	require.NoError(t, err)
	require.Equal(t, 0, cnt)
}

// ---------------------------------------------------------------------------
// Goship provider tests
// ---------------------------------------------------------------------------

// goshipTestSetup seeds the minimum rows required for GoshipProvider tests:
// a brand WITH a primary brand_address (city_code + district_code set),
// a customer address with location codes, and one cart item.
// Returns the testSetup plus the pool for ad-hoc queries.
func setupGoshipOrder(t *testing.T, qty int, price float64) testSetup {
	t.Helper()
	pool := testfixtures.MustPool(t)
	ctx := context.Background()

	customer := testfixtures.SeedCustomer(t, pool)
	addr := testfixtures.SeedCustomerAddress(t, pool, customer.ID, testfixtures.CustomerAddressOpts{IsDefault: true})
	brand := testfixtures.SeedBrand(t, pool, uuid.Nil)

	// Seed a primary brand address with city/district codes so GoshipProvider
	// can read the pickup location.
	testfixtures.SeedBrandAddress(t, pool, brand.ID, "100000", "100000100")

	cat := testfixtures.SeedCategory(t, pool)
	prod := testfixtures.SeedProduct(t, pool, brand.ID, cat.ID, "active")
	variantID := testfixtures.SeedVariant(t, pool, prod.ID, "M", "Black", price, qty+5)

	testfixtures.SeedCartItem(t, pool, customer.ID, variantID, qty, price)

	// Build OrderService with GoshipProvider (mock client) + brand address repo.
	brandAddrRepo := brandrepo.NewAddressPG(pool)
	goshipProv := provider.NewGoshipProvider(
		goship.NewMockClient(),
		brandAddrRepo,
		weight.Defaults{WeightG: 500, LengthCM: 20, WidthCM: 15, HeightCM: 10},
	)
	svc := service.NewOrderService(
		pool,
		orderrepo.NewOrderPG(pool),
		orderrepo.NewSubOrderPG(pool),
		orderrepo.NewOrderItemPG(pool),
		paymentrepo.NewPaymentPG(pool),
		productrepo.NewVariantPG(pool),
		customeraddrrepo.NewAddressPG(pool),
		authrepo.NewUserPG(pool),
		goshipProv,
		payos.NewMockClient(""),
		service.Config{
			ReservationTimeout: 30 * time.Minute,
			PayosReturnURL:     "http://ret",
			PayosCancelURL:     "http://can",
		},
	)
	_ = ctx
	return testSetup{
		UserID:    customer.ID,
		AddrID:    addr.ID,
		BrandID:   brand.ID,
		ProductID: prod.ID,
		VariantID: variantID,
		Svc:       svc,
		Pool:      pool,
	}
}

func TestPlaceOrder_Goship_StoresChosenCarrierFee(t *testing.T) {
	// qty=1 variant, default weight 500g → ceil(500/1000)=1 kg
	// ghnv3 fee = 15000 + 5000*1 = 20000 VND
	const qty = 1
	const price = 100000.0
	const expectedGhnFee int64 = 20000

	s := setupGoshipOrder(t, qty, price)
	ctx := context.Background()

	_, _, err := s.Svc.PlaceOrder(ctx, s.UserID, domain.PlaceOrderReq{
		AddressID:     s.AddrID,
		PaymentMethod: domain.PaymentMethodCOD,
		ShippingSelections: []domain.ShippingSelection{
			{BrandID: s.BrandID, Carrier: "ghnv3"},
		},
	})
	require.NoError(t, err)

	// Verify shipping_carrier and shipping_fee_vnd persisted in sub_orders.
	var carrier string
	var fee int64
	err = s.Pool.QueryRow(ctx,
		`SELECT so.shipping_carrier, so.shipping_fee_vnd
		   FROM sub_orders so
		   JOIN orders o ON o.id = so.order_id
		  WHERE o.user_id = $1
		  ORDER BY so.created_at DESC
		  LIMIT 1`,
		s.UserID).Scan(&carrier, &fee)
	require.NoError(t, err)
	require.Equal(t, "ghnv3", carrier)
	require.Equal(t, expectedGhnFee, fee)
}

func TestPlaceOrder_Goship_UnknownCarrier(t *testing.T) {
	s := setupGoshipOrder(t, 1, 100000)
	ctx := context.Background()

	_, _, err := s.Svc.PlaceOrder(ctx, s.UserID, domain.PlaceOrderReq{
		AddressID:     s.AddrID,
		PaymentMethod: domain.PaymentMethodCOD,
		ShippingSelections: []domain.ShippingSelection{
			{BrandID: s.BrandID, Carrier: "ghn"}, // "ghn" not returned by mock
		},
	})
	require.ErrorIs(t, err, domain.ErrCarrierUnavailable)
}
