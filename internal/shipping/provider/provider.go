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
	WeightG   *int
	LengthCM  *int
	WidthCM   *int
	HeightCM  *int
}

type CalcReq struct {
	BrandID    uuid.UUID
	ToAddress  ShippingAddress
	ToCityCode *string
	ToDistrict *string
	CODVND     int64 // carrier-collected amount (grand total for COD, 0 for PayOS)
	AmountVND  int64 // declared goods value (sub-order subtotal)
	Items      []CalcItem
}

type ShippingProvider interface {
	Quote(ctx context.Context, r CalcReq) ([]shippingdomain.ShippingOption, error)
}
