// internal/order/domain/order_test.go
package domain_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/order/domain"
)

func TestCanCustomerCancel_PayosUnpaid_Allowed(t *testing.T) {
	o := &domain.Order{
		PaymentMethod: domain.PaymentMethodPayos,
		PaymentStatus: domain.PaymentStatusPending,
		Status:        domain.OrderStatusPendingPayment,
		SubOrders:     []domain.SubOrder{{Status: domain.SubOrderStatusPending}},
	}
	d := o.CanCustomerCancel()
	require.True(t, d.Allowed)
}

func TestCanCustomerCancel_PayosPaid_BlockedPaidNotSupported(t *testing.T) {
	o := &domain.Order{
		PaymentMethod: domain.PaymentMethodPayos,
		PaymentStatus: domain.PaymentStatusPaid,
		Status:        domain.OrderStatusProcessing,
		SubOrders:     []domain.SubOrder{{Status: domain.SubOrderStatusPending}},
	}
	d := o.CanCustomerCancel()
	require.False(t, d.Allowed)
	require.Equal(t, "paid_not_supported", d.Reason)
}

func TestCanCustomerCancel_CODPending_Allowed(t *testing.T) {
	o := &domain.Order{
		PaymentMethod: domain.PaymentMethodCOD,
		PaymentStatus: domain.PaymentStatusPending,
		Status:        domain.OrderStatusProcessing,
		SubOrders:     []domain.SubOrder{{Status: domain.SubOrderStatusPending}},
	}
	d := o.CanCustomerCancel()
	require.True(t, d.Allowed)
}

func TestCanCustomerCancel_AnySubOrderConfirmed_Blocked(t *testing.T) {
	o := &domain.Order{
		PaymentMethod: domain.PaymentMethodCOD,
		PaymentStatus: domain.PaymentStatusPending,
		Status:        domain.OrderStatusProcessing,
		SubOrders: []domain.SubOrder{
			{Status: domain.SubOrderStatusPending},
			{Status: domain.SubOrderStatusConfirmed},
		},
	}
	d := o.CanCustomerCancel()
	require.False(t, d.Allowed)
	require.Equal(t, "already_shipped", d.Reason)
}

func TestCanCustomerCancel_AlreadyCancelled(t *testing.T) {
	o := &domain.Order{Status: domain.OrderStatusCancelled}
	d := o.CanCustomerCancel()
	require.False(t, d.Allowed)
	require.Equal(t, "already_cancelled", d.Reason)
}

func TestCanCustomerCancel_AlreadyCompleted(t *testing.T) {
	o := &domain.Order{Status: domain.OrderStatusCompleted}
	d := o.CanCustomerCancel()
	require.False(t, d.Allowed)
	require.Equal(t, "already_completed", d.Reason)
}

func TestPaymentMethodValid(t *testing.T) {
	require.True(t, domain.PaymentMethodCOD.Valid())
	require.True(t, domain.PaymentMethodPayos.Valid())
	require.False(t, domain.PaymentMethod("bitcoin").Valid())
}

// silence unused
var _ = uuid.New
