// internal/order/domain/enums.go
package domain

type PaymentMethod string

const (
	PaymentMethodCOD   PaymentMethod = "cod"
	PaymentMethodPayos PaymentMethod = "payos"
)

func (m PaymentMethod) Valid() bool {
	return m == PaymentMethodCOD || m == PaymentMethodPayos
}

type PaymentStatus string

const (
	PaymentStatusPending   PaymentStatus = "pending"
	PaymentStatusPaid      PaymentStatus = "paid"
	PaymentStatusFailed    PaymentStatus = "failed"
	PaymentStatusCancelled PaymentStatus = "cancelled"
	PaymentStatusExpired   PaymentStatus = "expired" // payments table only
)

type OrderStatus string

const (
	OrderStatusPendingPayment OrderStatus = "pending_payment"
	OrderStatusProcessing     OrderStatus = "processing"
	OrderStatusCancelled      OrderStatus = "cancelled"
	OrderStatusCompleted      OrderStatus = "completed"
)

type SubOrderStatus string

const (
	SubOrderStatusPending   SubOrderStatus = "pending"
	SubOrderStatusConfirmed SubOrderStatus = "confirmed"
	SubOrderStatusPreparing SubOrderStatus = "preparing"
	SubOrderStatusShipped   SubOrderStatus = "shipped"
	SubOrderStatusDelivered SubOrderStatus = "delivered"
	SubOrderStatusCancelled SubOrderStatus = "cancelled"
)
