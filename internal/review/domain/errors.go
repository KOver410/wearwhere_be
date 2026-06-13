package domain

import (
	"net/http"

	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

func ErrProductNotFound() *httpx.AppError {
	return httpx.NewAppError(http.StatusNotFound, "PRODUCT_NOT_FOUND", "Product not found")
}

func ErrNotVerifiedPurchase() *httpx.AppError {
	return httpx.NewAppError(http.StatusForbidden, "NOT_VERIFIED_PURCHASE", "You can only review a product you have received")
}

func ErrReviewExists() *httpx.AppError {
	return httpx.NewAppError(http.StatusConflict, "REVIEW_EXISTS", "You have already reviewed this product")
}

func ErrReviewNotFound() *httpx.AppError {
	return httpx.NewAppError(http.StatusNotFound, "REVIEW_NOT_FOUND", "Review not found")
}

func ErrForbidden() *httpx.AppError {
	return httpx.NewAppError(http.StatusForbidden, "FORBIDDEN", "You can only modify your own review")
}
