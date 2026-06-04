package domain

import (
	"errors"
	"net/http"

	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

var (
	ErrBrandNotFound   = httpx.NewAppError(http.StatusNotFound, "BRAND_NOT_FOUND", "Brand not found")
	ErrAddressNotFound = httpx.NewAppError(http.StatusNotFound, "ADDRESS_NOT_FOUND", "Address not found")
	ErrNoBrandOwned    = httpx.NewAppError(http.StatusForbidden, "NO_BRAND_OWNED", "User does not own a brand")
	ErrBrandSuspended  = httpx.NewAppError(http.StatusForbidden, "BRAND_SUSPENDED", "Brand is suspended")
	ErrSlugTaken       = httpx.NewAppError(http.StatusConflict, "SLUG_TAKEN", "Slug is already in use")
	ErrInvalidLocation = errors.New("invalid location: district/ward does not belong to parent")
)
