// internal/order/domain/dto.go
package domain

import (
	"time"

	"github.com/google/uuid"
)

// CheckoutPreviewItem is a single item in the preview response.
type CheckoutPreviewItem struct {
	VariantID    uuid.UUID `json:"variant_id"`
	ProductID    uuid.UUID `json:"product_id"`
	ProductName  string    `json:"product_name"`
	VariantLabel string    `json:"variant_label"`
	ImageURL     *string   `json:"image_url"`
	Qty          int       `json:"qty"`
	UnitPriceVND int64     `json:"unit_price_vnd"`
	LineTotalVND int64     `json:"line_total_vnd"`
	AvailableQty int       `json:"available_qty"`
}

type CheckoutPreviewSubOrder struct {
	Brand          BrandRef              `json:"brand"`
	Items          []CheckoutPreviewItem `json:"items"`
	SubtotalVND    int64                 `json:"subtotal_vnd"`
	ShippingFeeVND int64                 `json:"shipping_fee_vnd"`
	TotalVND       int64                 `json:"total_vnd"`
}

type BrandRef struct {
	ID   uuid.UUID `json:"id"`
	Slug string    `json:"slug"`
	Name string    `json:"name"`
}

type CheckoutPreviewResp struct {
	CartEmpty        bool                      `json:"cart_empty"`
	Address          *ShippingAddress          `json:"address,omitempty"`
	SubOrders        []CheckoutPreviewSubOrder `json:"sub_orders"`
	SubtotalVND      int64                     `json:"subtotal_vnd"`
	ShippingTotalVND int64                     `json:"shipping_total_vnd"`
	GrandTotalVND    int64                     `json:"grand_total_vnd"`
	MinOrderValueVND int64                     `json:"min_order_value_vnd"`
	MeetsMinOrder    bool                      `json:"meets_min_order"`
	Warnings         []string                  `json:"warnings"`
}

type PlaceOrderReq struct {
	AddressID     uuid.UUID     `json:"address_id" binding:"required"`
	PaymentMethod PaymentMethod `json:"payment_method" binding:"required"`
	Notes         string        `json:"notes" binding:"max=500"`
}

type PaymentResp struct {
	ID          uuid.UUID     `json:"id"`
	Method      PaymentMethod `json:"method"`
	Status      PaymentStatus `json:"status"`
	AmountVND   int64         `json:"amount_vnd"`
	CheckoutURL *string       `json:"checkout_url"`
	QRCode      *string       `json:"qr_code"`
	ExpiredAt   *time.Time    `json:"expired_at"`
}

type PlaceOrderResp struct {
	Order   OrderResp   `json:"order"`
	Payment PaymentResp `json:"payment"`
}

type OrderItemResp struct {
	ID           uuid.UUID `json:"id"`
	VariantID    uuid.UUID `json:"variant_id"`
	ProductID    uuid.UUID `json:"product_id"`
	ProductName  string    `json:"product_name"`
	VariantLabel string    `json:"variant_label"`
	ImageURL     *string   `json:"image_url"`
	Qty          int       `json:"qty"`
	UnitPriceVND int64     `json:"unit_price_vnd"`
	LineTotalVND int64     `json:"line_total_vnd"`
}

type SubOrderResp struct {
	ID             uuid.UUID       `json:"id"`
	Brand          BrandRef        `json:"brand"`
	SubtotalVND    int64           `json:"subtotal_vnd"`
	ShippingFeeVND int64           `json:"shipping_fee_vnd"`
	TotalVND       int64           `json:"total_vnd"`
	Status         SubOrderStatus  `json:"status"`
	TrackingNo     *string         `json:"tracking_no"`
	Items          []OrderItemResp `json:"items"`
}

type OrderResp struct {
	ID              uuid.UUID       `json:"id"`
	OrderNo         string          `json:"order_no"`
	Status          OrderStatus     `json:"status"`
	PaymentMethod   PaymentMethod   `json:"payment_method"`
	PaymentStatus   PaymentStatus   `json:"payment_status"`
	SubtotalVND     int64           `json:"subtotal_vnd"`
	ShippingTotalVND int64          `json:"shipping_total_vnd"`
	GrandTotalVND   int64           `json:"grand_total_vnd"`
	ShippingAddress ShippingAddress `json:"shipping_address"`
	Notes           string          `json:"notes"`
	CancelReason    string          `json:"cancel_reason,omitempty"`
	SubOrders       []SubOrderResp  `json:"sub_orders"`
	CreatedAt       time.Time       `json:"created_at"`
	PaidAt          *time.Time      `json:"paid_at"`
	CancelledAt     *time.Time      `json:"cancelled_at"`
}

type OrderListItem struct {
	ID             uuid.UUID     `json:"id"`
	OrderNo        string        `json:"order_no"`
	Status         OrderStatus   `json:"status"`
	PaymentMethod  PaymentMethod `json:"payment_method"`
	PaymentStatus  PaymentStatus `json:"payment_status"`
	GrandTotalVND  int64         `json:"grand_total_vnd"`
	ItemCount      int           `json:"item_count"`
	BrandCount     int           `json:"brand_count"`
	FirstItemImage *string       `json:"first_item_image"`
	FirstItemName  string        `json:"first_item_name"`
	CreatedAt      time.Time     `json:"created_at"`
}

type OrderListResp struct {
	Data       []OrderListItem `json:"data"`
	Page       int             `json:"page"`
	PageSize   int             `json:"page_size"`
	Total      int             `json:"total"`
	TotalPages int             `json:"total_pages"`
}

type CancelOrderReq struct {
	Reason string `json:"reason" binding:"max=200"`
}
