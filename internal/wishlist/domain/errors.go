package domain

import (
	"net/http"

	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

// ErrProductNotFound mirrors the product module's not-found semantic so
// wishlist add against an unknown/inactive product returns the same code.
var ErrProductNotFound = httpx.NewAppError(
	http.StatusNotFound,
	"PRODUCT_NOT_FOUND",
	"Product not found or unavailable",
)
