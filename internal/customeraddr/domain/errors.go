package domain

import (
	"net/http"

	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

var (
	ErrAddressNotFound = httpx.NewAppError(http.StatusNotFound, "ADDRESS_NOT_FOUND", "Address not found")
	ErrInvalidPhone    = httpx.NewAppError(http.StatusBadRequest, "INVALID_PHONE", "Phone must be in E.164 format")
)
