// internal/order/domain/order.go
package domain

import (
	"time"

	"github.com/google/uuid"
)

type ShippingAddress struct {
	Recipient    string  `json:"recipient"`
	Phone        string  `json:"phone"`
	Line1        string  `json:"line1"`
	Ward         string  `json:"ward"`
	District     string  `json:"district"`
	City         string  `json:"city"`
	CityCode     *string `json:"city_code,omitempty"`
	DistrictCode *string `json:"district_code,omitempty"`
	WardCode     *string `json:"ward_code,omitempty"`
}

type Order struct {
	ID               uuid.UUID
	UserID           uuid.UUID
	OrderNo          string
	SubtotalVND      int64
	ShippingTotalVND int64
	GrandTotalVND    int64
	PaymentMethod    PaymentMethod
	PaymentStatus    PaymentStatus
	Status           OrderStatus
	ShippingAddress  ShippingAddress
	Notes            string
	CancelReason     string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	PaidAt           *time.Time
	CancelledAt      *time.Time

	SubOrders []SubOrder // optional, populated by Detail queries
}

type SubOrder struct {
	ID               uuid.UUID
	OrderID          uuid.UUID
	BrandID          uuid.UUID
	BrandSlug        string // joined view
	BrandName        string // joined view
	SubtotalVND      int64
	ShippingFeeVND   int64
	TotalVND         int64
	Status           SubOrderStatus
	TrackingNo       *string
	ShippingCarrier  *string
	ShippingProvider   *string
	ShippingCostVND    *int64
	GoshipShipmentCode *string
	TrackingURL        *string
	ShippingStatusText *string
	ConfirmedAt        *time.Time
	ShippedAt        *time.Time
	DeliveredAt      *time.Time
	CancelledAt      *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time

	Items []OrderItem // optional, populated by Detail queries
}

type OrderItem struct {
	ID           uuid.UUID
	SubOrderID   uuid.UUID
	VariantID    uuid.UUID
	ProductID    uuid.UUID
	ProductName  string
	VariantLabel string
	ImageURL     *string
	Qty          int
	UnitPriceVND int64
	LineTotalVND int64
	CreatedAt    time.Time
}

// CancelDecision encodes whether a customer-initiated cancel is allowed.
type CancelDecision struct {
	Allowed bool
	Reason  string // subcode: "" (allowed), "paid_not_supported", "already_shipped",
	// "already_cancelled", "already_completed"
}

// CanCustomerCancel implements the rule table from spec §5.3.
// Sprint 3: COD pending OR PayOS unpaid → allowed; paid PayOS → defer Sprint 4.
func (o *Order) CanCustomerCancel() CancelDecision {
	switch o.Status {
	case OrderStatusCancelled:
		return CancelDecision{Allowed: false, Reason: "already_cancelled"}
	case OrderStatusCompleted:
		return CancelDecision{Allowed: false, Reason: "already_completed"}
	}

	// Any sub_order advanced beyond pending → block (Sprint 4 will handle paid/shipped flows)
	for _, so := range o.SubOrders {
		if so.Status != SubOrderStatusPending {
			return CancelDecision{Allowed: false, Reason: "already_shipped"}
		}
	}

	if o.PaymentMethod == PaymentMethodPayos && o.PaymentStatus == PaymentStatusPaid {
		return CancelDecision{Allowed: false, Reason: "paid_not_supported"}
	}

	return CancelDecision{Allowed: true}
}
