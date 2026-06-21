package domain

import (
	"time"

	"github.com/google/uuid"
)

// CreatePromoReq is the admin payload to create a promo code.
type CreatePromoReq struct {
	Code             string       `json:"code" binding:"required,max=40"`
	Description      string       `json:"description" binding:"max=255"`
	DiscountType     DiscountType `json:"discount_type" binding:"required"`
	DiscountValue    int64        `json:"discount_value" binding:"required"`
	MaxDiscountVND   *int64       `json:"max_discount_vnd"`
	MinOrderValueVND int64        `json:"min_order_value_vnd"`
	StartsAt         *time.Time   `json:"starts_at"`
	EndsAt           *time.Time   `json:"ends_at"`
	IsActive         *bool        `json:"is_active"`
}

// UpdatePromoReq patches mutable fields. Nil fields are left unchanged.
type UpdatePromoReq struct {
	Description      *string    `json:"description"`
	MaxDiscountVND   *int64     `json:"max_discount_vnd"`
	MinOrderValueVND *int64     `json:"min_order_value_vnd"`
	StartsAt         *time.Time `json:"starts_at"`
	EndsAt           *time.Time `json:"ends_at"`
	IsActive         *bool      `json:"is_active"`
}

// PromoResp is the wire representation of a promo code.
type PromoResp struct {
	ID               uuid.UUID    `json:"id"`
	Code             string       `json:"code"`
	Description      string       `json:"description"`
	DiscountType     DiscountType `json:"discount_type"`
	DiscountValue    int64        `json:"discount_value"`
	MaxDiscountVND   *int64       `json:"max_discount_vnd"`
	MinOrderValueVND int64        `json:"min_order_value_vnd"`
	StartsAt         time.Time    `json:"starts_at"`
	EndsAt           *time.Time   `json:"ends_at"`
	IsActive         bool         `json:"is_active"`
	CreatedAt        time.Time    `json:"created_at"`
	UpdatedAt        time.Time    `json:"updated_at"`
}

// PromoListResp is a paginated list of promo codes.
type PromoListResp struct {
	Data       []PromoResp `json:"data"`
	Page       int         `json:"page"`
	PageSize   int         `json:"page_size"`
	Total      int         `json:"total"`
	TotalPages int         `json:"total_pages"`
}

// ToResp maps a PromoCode to its wire DTO.
func ToResp(p *PromoCode) PromoResp {
	return PromoResp{
		ID:               p.ID,
		Code:             p.Code,
		Description:      p.Description,
		DiscountType:     p.DiscountType,
		DiscountValue:    p.DiscountValue,
		MaxDiscountVND:   p.MaxDiscountVND,
		MinOrderValueVND: p.MinOrderValueVND,
		StartsAt:         p.StartsAt,
		EndsAt:           p.EndsAt,
		IsActive:         p.IsActive,
		CreatedAt:        p.CreatedAt,
		UpdatedAt:        p.UpdatedAt,
	}
}
