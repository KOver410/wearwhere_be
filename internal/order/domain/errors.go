// internal/order/domain/errors.go
package domain

import "errors"

var (
	ErrOrderNotFound           = errors.New("order: not found")
	ErrCartEmpty               = errors.New("order: cart is empty")
	ErrMinOrderValue           = errors.New("order: subtotal below 50000 VND minimum")
	ErrInsufficientStock       = errors.New("order: insufficient stock for variant")
	ErrVariantUnavailable      = errors.New("order: variant unavailable")
	ErrAddressNotFound         = errors.New("order: shipping address not found or not owned")
	ErrInvalidPaymentMethod    = errors.New("order: invalid payment method")
	ErrCancelNotAllowed        = errors.New("order: cannot be cancelled in current state")
	ErrCancelPaidNotSupported  = errors.New("order: paid order cancellation deferred to Sprint 4")
	ErrWebhookSignatureInvalid = errors.New("order: invalid webhook signature")
	ErrPayosLinkCreate         = errors.New("order: failed to create PayOS payment link")
	ErrIDOR                    = errors.New("order: resource not owned by user")
	ErrAddressIncomplete       = errors.New("shipping address is missing city/district/ward code")
	ErrCarrierUnavailable      = errors.New("selected carrier is no longer available for this route")
	ErrCarrierNotSelected      = errors.New("no shipping carrier selected for one or more brands")
	ErrShippingUnavailable     = errors.New("shipping service temporarily unavailable")
)

const MinOrderValueVND int64 = 50000
