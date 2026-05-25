package provider

import (
	"context"

	"github.com/google/uuid"

	shippingdomain "github.com/wearwhere/wearwhere_be/internal/shipping/domain"
)

// ShippingAddress mirrors order.ShippingAddress without importing order/domain
// to avoid an import cycle (order will import shipping/provider).
type ShippingAddress struct {
	Recipient string
	Phone     string
	Line1     string
	Ward      string
	District  string
	City      string
}

type CalcItem struct {
	VariantID uuid.UUID
	ProductID uuid.UUID
	Qty       int
	WeightG   int // 0 in Sprint 3 — variant weight column not yet added
}

type CalcReq struct {
	BrandID   uuid.UUID
	ToAddress ShippingAddress
	Items     []CalcItem
}

type ShippingProvider interface {
	Calculate(ctx context.Context, r CalcReq) (*shippingdomain.FeeQuote, error)
}
