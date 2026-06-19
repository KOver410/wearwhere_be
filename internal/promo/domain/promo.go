// Package domain holds the promo-code (mã giảm giá) entities and rules.
package domain

import (
	"time"

	"github.com/google/uuid"
)

// DiscountType is how a promo code reduces the order subtotal.
type DiscountType string

const (
	// DiscountTypePercentage reduces by discount_value percent (1..100).
	DiscountTypePercentage DiscountType = "percentage"
	// DiscountTypeFixed reduces by discount_value VND.
	DiscountTypeFixed DiscountType = "fixed"
)

func (t DiscountType) Valid() bool {
	return t == DiscountTypePercentage || t == DiscountTypeFixed
}

// PromoCode mirrors a row of the promo_codes table.
type PromoCode struct {
	ID               uuid.UUID
	Code             string
	Description      string
	DiscountType     DiscountType
	DiscountValue    int64 // percent (1..100) when percentage, else VND amount
	MaxDiscountVND   *int64
	MinOrderValueVND int64
	StartsAt         time.Time
	EndsAt           *time.Time
	IsActive         bool
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// ComputeDiscount returns the VND discount this code yields for the given
// subtotal. The result is clamped at subtotalVND so the discount never exceeds
// the goods value (shipping is never discounted).
func (p *PromoCode) ComputeDiscount(subtotalVND int64) int64 {
	if subtotalVND <= 0 {
		return 0
	}
	var d int64
	switch p.DiscountType {
	case DiscountTypePercentage:
		d = subtotalVND * p.DiscountValue / 100
		if p.MaxDiscountVND != nil && d > *p.MaxDiscountVND {
			d = *p.MaxDiscountVND
		}
	case DiscountTypeFixed:
		d = p.DiscountValue
	}
	if d > subtotalVND {
		d = subtotalVND
	}
	if d < 0 {
		d = 0
	}
	return d
}

// ValidateResult is the outcome of a successful promo validation.
type ValidateResult struct {
	PromoID       uuid.UUID
	Code          string
	DiscountType  DiscountType
	DiscountValue int64
	DiscountVND   int64
}
