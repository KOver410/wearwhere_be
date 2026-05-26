// internal/payment/domain/payment.go
package domain

import (
	"time"

	"github.com/google/uuid"

	orderdomain "github.com/wearwhere/wearwhere_be/internal/order/domain"
)

type Payment struct {
	ID                 uuid.UUID
	OrderID            uuid.UUID
	AmountVND          int64
	Method             orderdomain.PaymentMethod
	Status             orderdomain.PaymentStatus
	PayosOrderCode     *int64
	PayosPaymentLinkID *string
	PayosCheckoutURL   *string
	PayosQRCode        *string
	ExpiredAt          *time.Time
	PaidAt             *time.Time
	FailureReason      *string
	RawWebhookPayload  []byte
	CreatedAt          time.Time
	UpdatedAt          time.Time
}
