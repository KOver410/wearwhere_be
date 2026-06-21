//go:build integration

package jobs_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	authrepo "github.com/wearwhere/wearwhere_be/internal/auth/repo"
	brandrepo "github.com/wearwhere/wearwhere_be/internal/brand/repo"
	customeraddrrepo "github.com/wearwhere/wearwhere_be/internal/customeraddr/repo"
	"github.com/wearwhere/wearwhere_be/internal/jobs"
	"github.com/wearwhere/wearwhere_be/internal/order/domain"
	orderrepo "github.com/wearwhere/wearwhere_be/internal/order/repo"
	"github.com/wearwhere/wearwhere_be/internal/order/service"
	"github.com/wearwhere/wearwhere_be/internal/payment/payos"
	paymentrepo "github.com/wearwhere/wearwhere_be/internal/payment/repo"
	productrepo "github.com/wearwhere/wearwhere_be/internal/product/repo"
	"github.com/wearwhere/wearwhere_be/internal/shipping/provider"
	"github.com/wearwhere/wearwhere_be/internal/testfixtures"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

// jobSetup holds everything needed to run cleanup-job integration tests.
type jobSetup struct {
	Pool      *pgxpool.Pool
	UserID    uuid.UUID
	AddrID    uuid.UUID
	BrandID   uuid.UUID
	VariantID uuid.UUID
	Svc       *service.OrderService
}

// buildJobSvc constructs an OrderService backed by pool, using mock PayOS.
func buildJobSvc(pool *pgxpool.Pool) *service.OrderService {
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
		nil, // promo not exercised in reservation-cleanup tests
		service.Config{
			ReservationTimeout: 30 * time.Minute,
			PayosReturnURL:     "http://ret",
			PayosCancelURL:     "http://can",
		},
	)
}

// setupJobTest seeds minimum required rows for a cleanup-job test.
// It calls testfixtures.Clean first so residual rows from previous test runs
// do not interfere with counts or status assertions.
func setupJobTest(t *testing.T) jobSetup {
	t.Helper()
	pool := testfixtures.MustPool(t)
	testfixtures.Clean(t, pool)
	ctx := context.Background()

	customer := testfixtures.SeedCustomer(t, pool)
	addr := testfixtures.SeedCustomerAddress(t, pool, customer.ID, testfixtures.CustomerAddressOpts{IsDefault: true})
	brand := testfixtures.SeedBrand(t, pool, uuid.Nil)
	cat := testfixtures.SeedCategory(t, pool)
	prod := testfixtures.SeedProduct(t, pool, brand.ID, cat.ID, "active")
	variantID := testfixtures.SeedVariant(t, pool, prod.ID, "M", "Black", 100000, 10)

	_, _ = pool.Exec(ctx,
		`UPDATE brands SET shipping_flat_fee_vnd = 30000 WHERE id = $1`, brand.ID)

	testfixtures.SeedCartItem(t, pool, customer.ID, variantID, 1, 100000)

	return jobSetup{
		Pool:      pool,
		UserID:    customer.ID,
		AddrID:    addr.ID,
		BrandID:   brand.ID,
		VariantID: variantID,
		Svc:       buildJobSvc(pool),
	}
}

// buildJob constructs a ReservationCleanupJob wired to pool.
func buildJob(pool *pgxpool.Pool, timeoutMin int) *jobs.ReservationCleanupJob {
	return jobs.NewReservationCleanupJob(
		pool,
		orderrepo.NewOrderPG(pool),
		orderrepo.NewSubOrderPG(pool),
		orderrepo.NewOrderItemPG(pool),
		paymentrepo.NewPaymentPG(pool),
		productrepo.NewVariantPG(pool),
		timeoutMin,
	)
}

// TestCleanupOnce_ExpiresOldPendingPayments places a PayOS order, backdates the
// payment's created_at by 5 minutes, then runs cleanup with timeoutMin=1 and
// expects exactly 1 order to be expired. It verifies:
//   - order status = cancelled
//   - payment status = expired
//   - stock_qty unchanged, reserved_qty = 0
func TestCleanupOnce_ExpiresOldPendingPayments(t *testing.T) {
	s := setupJobTest(t)
	ctx := context.Background()

	// Place PayOS order — this reserves stock and creates a pending payment.
	_, _, err := s.Svc.PlaceOrder(ctx, s.UserID, domain.PlaceOrderReq{
		AddressID:          s.AddrID,
		PaymentMethod:      domain.PaymentMethodPayos,
		ShippingSelections: []domain.ShippingSelection{{BrandID: s.BrandID, Carrier: "flat"}},
	})
	require.NoError(t, err)

	// Verify stock is reserved.
	stock, reserved := testfixtures.GetVariantStock(t, s.Pool, s.VariantID)
	require.Equal(t, 10, stock)
	require.Equal(t, 1, reserved)

	// Backdate the payment's created_at by 5 minutes to trigger cleanup.
	_, err = s.Pool.Exec(ctx,
		`UPDATE payments SET created_at = NOW() - INTERVAL '5 minutes'
		  WHERE method = 'payos' AND status = 'pending'`)
	require.NoError(t, err)

	// Run cleanup with timeout = 1 minute — the payment is 5 min old, so it expires.
	job := buildJob(s.Pool, 1)
	n, err := job.CleanupOnce(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, n, "expected 1 expired order")

	// Verify order is cancelled.
	var orderStatus, paymentStatus string
	err = s.Pool.QueryRow(ctx,
		`SELECT o.status, o.payment_status
		   FROM orders o
		   JOIN payments p ON p.order_id = o.id
		  WHERE p.method = 'payos'`).Scan(&orderStatus, &paymentStatus)
	require.NoError(t, err)
	require.Equal(t, string(domain.OrderStatusCancelled), orderStatus)
	require.Equal(t, string(domain.PaymentStatusCancelled), paymentStatus)

	// Verify payment row is expired.
	var pmtStatus string
	err = s.Pool.QueryRow(ctx,
		`SELECT status FROM payments WHERE method = 'payos'`).Scan(&pmtStatus)
	require.NoError(t, err)
	require.Equal(t, "expired", pmtStatus)

	// Verify stock_qty unchanged and reserved_qty = 0.
	stockAfter, reservedAfter := testfixtures.GetVariantStock(t, s.Pool, s.VariantID)
	require.Equal(t, 10, stockAfter, "stock_qty should be unchanged")
	require.Equal(t, 0, reservedAfter, "reserved_qty should be 0 after cleanup")
}

// TestCleanupOnce_SkipsRecentPayments places a PayOS order and immediately runs
// cleanup with timeoutMin=30. Since the payment is only seconds old, it must
// NOT be expired.
func TestCleanupOnce_SkipsRecentPayments(t *testing.T) {
	s := setupJobTest(t)
	ctx := context.Background()

	// Place PayOS order.
	_, _, err := s.Svc.PlaceOrder(ctx, s.UserID, domain.PlaceOrderReq{
		AddressID:          s.AddrID,
		PaymentMethod:      domain.PaymentMethodPayos,
		ShippingSelections: []domain.ShippingSelection{{BrandID: s.BrandID, Carrier: "flat"}},
	})
	require.NoError(t, err)

	// Verify stock is reserved before cleanup.
	_, reserved := testfixtures.GetVariantStock(t, s.Pool, s.VariantID)
	require.Equal(t, 1, reserved)

	// Run cleanup with timeout = 30 minutes — the payment is only seconds old.
	job := buildJob(s.Pool, 30)
	n, err := job.CleanupOnce(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, n, "recent payment must not be expired")

	// Stock reservation must still be set.
	_, reservedAfter := testfixtures.GetVariantStock(t, s.Pool, s.VariantID)
	require.Equal(t, 1, reservedAfter, "reserved_qty should still be 1")

	// Payment status must still be pending.
	var pmtStatus string
	err = s.Pool.QueryRow(ctx,
		`SELECT status FROM payments WHERE method = 'payos'`).Scan(&pmtStatus)
	require.NoError(t, err)
	require.Equal(t, "pending", pmtStatus)
}
