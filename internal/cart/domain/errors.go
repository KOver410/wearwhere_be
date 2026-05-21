package domain

import (
	"net/http"

	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

var (
	ErrCartItemNotFound   = httpx.NewAppError(http.StatusNotFound,   "CART_ITEM_NOT_FOUND",   "Cart item not found")
	ErrVariantUnavailable = httpx.NewAppError(http.StatusConflict,   "VARIANT_UNAVAILABLE",   "Variant or product is no longer available")
	ErrOutOfStock         = httpx.NewAppError(http.StatusConflict,   "VARIANT_OUT_OF_STOCK",  "Insufficient stock for requested quantity")
	ErrQtyExceedsMax      = httpx.NewAppError(http.StatusBadRequest, "QTY_EXCEEDS_MAX",       "Quantity exceeds maximum allowed per item (10)")
)
