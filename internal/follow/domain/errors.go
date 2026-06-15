package domain

import (
	"net/http"

	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

func ErrCannotFollowSelf() *httpx.AppError {
	return httpx.NewAppError(http.StatusBadRequest, "CANNOT_FOLLOW_SELF", "You cannot follow yourself")
}

func ErrUserNotFound() *httpx.AppError {
	return httpx.NewAppError(http.StatusNotFound, "USER_NOT_FOUND", "User not found")
}

func ErrBrandNotFound() *httpx.AppError {
	return httpx.NewAppError(http.StatusNotFound, "BRAND_NOT_FOUND", "Brand not found")
}
