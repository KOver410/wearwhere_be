package domain

import (
	"net/http"

	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

func ErrCannotBlockSelf() *httpx.AppError {
	return httpx.NewAppError(http.StatusBadRequest, "CANNOT_BLOCK_SELF", "You cannot block yourself")
}

func ErrUserNotFound() *httpx.AppError {
	return httpx.NewAppError(http.StatusNotFound, "USER_NOT_FOUND", "User not found")
}
