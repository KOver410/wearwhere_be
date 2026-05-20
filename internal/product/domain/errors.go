package domain

import (
	"net/http"

	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

var (
	ErrProductNotFound       = httpx.NewAppError(http.StatusNotFound, "PRODUCT_NOT_FOUND", "Product not found")
	ErrVariantNotFound       = httpx.NewAppError(http.StatusNotFound, "VARIANT_NOT_FOUND", "Variant not found")
	ErrImageNotFound         = httpx.NewAppError(http.StatusNotFound, "IMAGE_NOT_FOUND", "Image not found")
	ErrCategoryNotFound      = httpx.NewAppError(http.StatusNotFound, "CATEGORY_NOT_FOUND", "Category not found")
	ErrSlugTaken             = httpx.NewAppError(http.StatusConflict, "SLUG_TAKEN", "Slug already in use")
	ErrVariantConflict       = httpx.NewAppError(http.StatusConflict, "VARIANT_CONFLICT", "Variant with this size+color already exists")
	ErrProductNotPublishable = httpx.NewAppError(http.StatusConflict, "PRODUCT_NOT_PUBLISHABLE", "Product needs at least 1 variant and 1 image to publish")
	ErrInvalidMIME           = httpx.NewAppError(http.StatusBadRequest, "INVALID_MIME", "Unsupported file type")
	ErrFileTooLarge          = httpx.NewAppError(http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE", "File exceeds maximum size")
	ErrTooManyFiles          = httpx.NewAppError(http.StatusBadRequest, "TOO_MANY_FILES", "Too many files in one request")
	ErrStorageError          = httpx.NewAppError(http.StatusBadGateway, "STORAGE_ERROR", "Storage backend failure")
)
