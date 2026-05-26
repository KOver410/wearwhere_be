//go:build integration

package repo_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/order/domain"
	orderrepo "github.com/wearwhere/wearwhere_be/internal/order/repo"
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

func seedOrderVal(userID uuid.UUID, orderNo string) *domain.Order {
	return &domain.Order{
		UserID:           userID,
		OrderNo:          orderNo,
		SubtotalVND:      100000,
		ShippingTotalVND: 30000,
		GrandTotalVND:    130000,
		PaymentMethod:    domain.PaymentMethodCOD,
		PaymentStatus:    domain.PaymentStatusPending,
		Status:           domain.OrderStatusProcessing,
		ShippingAddress: domain.ShippingAddress{
			Recipient: "An Nguyen", Phone: "0900000000",
			Line1: "1 ABC St", Ward: "P1", District: "Q1", City: "HCM",
		},
		Notes: "test",
	}
}

func TestOrderPG_CreateAndGet(t *testing.T) {
	tx := testfixtures.BeginTx(t, pool)
	user := testfixtures.SeedCustomer(t, tx)
	r := orderrepo.NewOrderPG(tx)
	ctx := context.Background()

	o := seedOrderVal(user.ID, "WW-20260524-AAAAAA")
	require.NoError(t, r.Create(ctx, tx, o))
	require.NotEqual(t, uuid.Nil, o.ID)

	got, err := r.GetByOrderNo(ctx, user.ID, "WW-20260524-AAAAAA")
	require.NoError(t, err)
	require.Equal(t, o.ID, got.ID)
	require.Equal(t, "An Nguyen", got.ShippingAddress.Recipient)
	require.Equal(t, "test", got.Notes)
}

func TestOrderPG_Create_DuplicateOrderNo(t *testing.T) {
	tx := testfixtures.BeginTx(t, pool)
	user := testfixtures.SeedCustomer(t, tx)
	r := orderrepo.NewOrderPG(tx)
	ctx := context.Background()

	a := seedOrderVal(user.ID, "WW-20260524-DUP")
	require.NoError(t, r.Create(ctx, tx, a))

	b := seedOrderVal(user.ID, "WW-20260524-DUP")
	err := r.Create(ctx, tx, b)
	require.ErrorIs(t, err, orderrepo.ErrOrderNoConflict)
}

func TestOrderPG_GetByOrderNo_OtherUser_NotFound(t *testing.T) {
	tx := testfixtures.BeginTx(t, pool)
	u1 := testfixtures.SeedCustomer(t, tx)
	u2 := testfixtures.SeedCustomer(t, tx)
	r := orderrepo.NewOrderPG(tx)
	ctx := context.Background()

	o := seedOrderVal(u1.ID, "WW-20260524-IDOR")
	require.NoError(t, r.Create(ctx, tx, o))

	_, err := r.GetByOrderNo(ctx, u2.ID, "WW-20260524-IDOR")
	require.ErrorIs(t, err, orderrepo.ErrNotFound)
}

func TestOrderPG_List_FilterByStatusAndPaginate(t *testing.T) {
	tx := testfixtures.BeginTx(t, pool)
	user := testfixtures.SeedCustomer(t, tx)
	r := orderrepo.NewOrderPG(tx)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		o := seedOrderVal(user.ID, "WW-20260524-X"+string(rune('A'+i))+"AAAA")
		require.NoError(t, r.Create(ctx, tx, o))
	}
	// Make one cancelled
	_, err := tx.Exec(ctx, `UPDATE orders SET status='cancelled' WHERE order_no LIKE 'WW-20260524-XA%'`)
	require.NoError(t, err)

	items, total, err := r.List(ctx, orderrepo.ListFilter{
		UserID:   user.ID,
		Statuses: []domain.OrderStatus{domain.OrderStatusProcessing},
		Page:     1, PageSize: 10,
	})
	require.NoError(t, err)
	require.Equal(t, 4, total) // 5 - 1 cancelled
	require.Len(t, items, 4)
}

func TestOrderPG_UpdateStatusOnPaid(t *testing.T) {
	tx := testfixtures.BeginTx(t, pool)
	user := testfixtures.SeedCustomer(t, tx)
	r := orderrepo.NewOrderPG(tx)
	ctx := context.Background()

	o := seedOrderVal(user.ID, "WW-20260524-PAID")
	o.Status = domain.OrderStatusPendingPayment
	require.NoError(t, r.Create(ctx, tx, o))

	require.NoError(t, r.UpdateStatusOnPaid(ctx, tx, o.ID))

	got, err := r.GetByID(ctx, o.ID)
	require.NoError(t, err)
	require.Equal(t, domain.OrderStatusProcessing, got.Status)
	require.Equal(t, domain.PaymentStatusPaid, got.PaymentStatus)
	require.NotNil(t, got.PaidAt)
}

func TestOrderPG_UpdateStatusOnCancel(t *testing.T) {
	tx := testfixtures.BeginTx(t, pool)
	user := testfixtures.SeedCustomer(t, tx)
	r := orderrepo.NewOrderPG(tx)
	ctx := context.Background()

	o := seedOrderVal(user.ID, "WW-20260524-CANC")
	require.NoError(t, r.Create(ctx, tx, o))

	require.NoError(t, r.UpdateStatusOnCancel(ctx, tx, o.ID, "user_cancel", domain.PaymentStatusCancelled))

	got, err := r.GetByID(ctx, o.ID)
	require.NoError(t, err)
	require.Equal(t, domain.OrderStatusCancelled, got.Status)
	require.Equal(t, "user_cancel", got.CancelReason)
	require.NotNil(t, got.CancelledAt)
}
