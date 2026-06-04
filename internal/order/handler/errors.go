package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/wearwhere/wearwhere_be/internal/order/domain"
	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

// refreshCartReason is the user-facing hint returned for stock/availability
// failures so the client knows to re-fetch the cart before retrying.
const refreshCartReason = "Refresh cart and retry"

// writeOrderError maps an error returned by the order/checkout service layer to
// the shared HTTP error envelope. Matching uses errors.Is so wrapped sentinels
// are handled. Unknown errors collapse to a generic 500 and never expose
// err.Error() to the client.
func writeOrderError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrCartEmpty):
		httpx.Error(c, http.StatusBadRequest, "CART_EMPTY", "Your cart is empty")
	case errors.Is(err, domain.ErrMinOrderValue):
		httpx.ErrorWithDetails(c, http.StatusBadRequest, "MIN_ORDER_VALUE",
			"Order subtotal is below the minimum required value",
			map[string]any{"min_value_vnd": domain.MinOrderValueVND})
	case errors.Is(err, domain.ErrAddressNotFound):
		httpx.Error(c, http.StatusNotFound, "ADDRESS_NOT_FOUND", "Shipping address not found")
	case errors.Is(err, domain.ErrInsufficientStock):
		httpx.ErrorWithDetails(c, http.StatusConflict, "INSUFFICIENT_STOCK",
			"One or more items are out of stock",
			map[string]any{"reason": refreshCartReason})
	case errors.Is(err, domain.ErrVariantUnavailable):
		httpx.ErrorWithDetails(c, http.StatusUnprocessableEntity, "VARIANT_UNAVAILABLE",
			"One or more items are no longer available",
			map[string]any{"reason": refreshCartReason})
	case errors.Is(err, domain.ErrInvalidPaymentMethod):
		httpx.Error(c, http.StatusBadRequest, "INVALID_PAYMENT_METHOD", "Invalid payment method")
	case errors.Is(err, domain.ErrPayosLinkCreate):
		httpx.Error(c, http.StatusBadGateway, "PAYOS_UNAVAILABLE", "Payment provider is unavailable")
	case errors.Is(err, domain.ErrOrderNotFound):
		httpx.Error(c, http.StatusNotFound, "ORDER_NOT_FOUND", "Order not found")
	case errors.Is(err, domain.ErrCancelPaidNotSupported):
		httpx.ErrorWithDetails(c, http.StatusConflict, "CANCEL_NOT_ALLOWED",
			"This order cannot be cancelled",
			map[string]any{"subcode": "paid_not_supported"})
	case errors.Is(err, domain.ErrCancelNotAllowed):
		httpx.ErrorWithDetails(c, http.StatusConflict, "CANCEL_NOT_ALLOWED",
			"This order cannot be cancelled in its current state",
			map[string]any{"subcode": "not_allowed"})
	default:
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
	}
}
