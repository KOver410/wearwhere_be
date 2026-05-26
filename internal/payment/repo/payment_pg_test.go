//go:build integration

package repo_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	orderdomain "github.com/wearwhere/wearwhere_be/internal/order/domain"
	orderrepo "github.com/wearwhere/wearwhere_be/internal/order/repo"
	paymentdomain "github.com/wearwhere/wearwhere_be/internal/payment/domain"
	paymentrepo "github.com/wearwhere/wearwhere_be/internal/payment/repo"
	"github.com/wearwhere/wearwhere_be/internal/testfixtures"
)

var pool *pgxpool.Pool

func TestMain(m *testing.M) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		panic("TEST_DATABASE_URL required")
	}
	var err error
	pool, err = pgxpool.New(context.Background(), dsn)
	if err != nil {
		panic(err)
	}
	defer pool.Close()
	os.Exit(m.Run())
}

func seedOrderForPayment(t *testing.T, ctx context.Context, tx testfixtures.DBTX, orderNo string) *orderdomain.Order {
	t.Helper()
	user := testfixtures.SeedCustomer(t, tx)
	o := &orderdomain.Order{
		UserID:           user.ID,
		OrderNo:          orderNo,
		SubtotalVND:      100000,
		ShippingTotalVND: 30000,
		GrandTotalVND:    130000,
		PaymentMethod:    orderdomain.PaymentMethodPayos,
		PaymentStatus:    orderdomain.PaymentStatusPending,
		Status:           orderdomain.OrderStatusPendingPayment,
		ShippingAddress: orderdomain.ShippingAddress{
			Recipient: "An Nguyen", Phone: "0900000000",
			Line1: "1 ABC St", Ward: "P1", District: "Q1", City: "HCM",
		},
	}
	orepo := orderrepo.NewOrderPG(tx)
	require.NoError(t, orepo.Create(ctx, tx, o))
	return o
}

func TestPaymentPG_CreateAndGet(t *testing.T) {
	tx := testfixtures.BeginTx(t, pool)
	ctx := context.Background()

	o := seedOrderForPayment(t, ctx, tx, "WW-20260524-PAYC")

	prepo := paymentrepo.NewPaymentPG(tx)
	code := int64(1700000000001)
	exp := time.Now().Add(30 * time.Minute)
	p := &paymentdomain.Payment{
		OrderID:        o.ID,
		AmountVND:      130000,
		Method:         orderdomain.PaymentMethodPayos,
		Status:         orderdomain.PaymentStatusPending,
		PayosOrderCode: &code,
		ExpiredAt:      &exp,
	}
	require.NoError(t, prepo.Create(ctx, tx, p))
	require.NotEqual(t, [16]byte{}, [16]byte(p.ID))

	got, err := prepo.GetByPayosOrderCode(ctx, code)
	require.NoError(t, err)
	require.Equal(t, p.ID, got.ID)
	require.Equal(t, o.ID, got.OrderID)
	require.Equal(t, int64(130000), got.AmountVND)
}

func TestPaymentPG_UpdateOnPaid_IdempotentOnSecondCall(t *testing.T) {
	tx := testfixtures.BeginTx(t, pool)
	ctx := context.Background()

	o := seedOrderForPayment(t, ctx, tx, "WW-20260524-IDEM")

	prepo := paymentrepo.NewPaymentPG(tx)
	code := int64(1700000000002)
	p := &paymentdomain.Payment{
		OrderID:        o.ID,
		AmountVND:      130000,
		Method:         orderdomain.PaymentMethodPayos,
		Status:         orderdomain.PaymentStatusPending,
		PayosOrderCode: &code,
	}
	require.NoError(t, prepo.Create(ctx, tx, p))

	require.NoError(t, prepo.UpdateOnPaid(ctx, tx, p.ID, []byte(`{"ok":1}`)))

	err := prepo.UpdateOnPaid(ctx, tx, p.ID, []byte(`{"ok":1}`))
	require.ErrorIs(t, err, paymentdomain.ErrIdempotent)
}
