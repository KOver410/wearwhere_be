package domain

import (
	"net/http"

	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

func ErrStoreNotFound() *httpx.AppError {
	return httpx.NewAppError(http.StatusNotFound, "STORE_NOT_FOUND", "Store not found")
}

func ErrDirectionsUnavailable() *httpx.AppError {
	return httpx.NewAppError(http.StatusBadGateway, "DIRECTIONS_UNAVAILABLE", "Could not compute directions")
}
