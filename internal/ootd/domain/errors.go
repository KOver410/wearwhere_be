package domain

import (
	"net/http"

	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

func ErrPostNotFound() *httpx.AppError {
	return httpx.NewAppError(http.StatusNotFound, "POST_NOT_FOUND", "Post not found")
}

func ErrCommentNotFound() *httpx.AppError {
	return httpx.NewAppError(http.StatusNotFound, "COMMENT_NOT_FOUND", "Comment not found")
}

func ErrForbidden() *httpx.AppError {
	return httpx.NewAppError(http.StatusForbidden, "FORBIDDEN", "You can only modify your own content")
}

func ErrNoPhotos() *httpx.AppError {
	return httpx.NewAppError(http.StatusBadRequest, "NO_PHOTOS", "A post needs 1 to 10 photos")
}

func ErrTooManyPhotos() *httpx.AppError {
	return httpx.NewAppError(http.StatusBadRequest, "TOO_MANY_PHOTOS", "A post allows at most 10 photos")
}

func ErrFileTooLarge() *httpx.AppError {
	return httpx.NewAppError(http.StatusBadRequest, "FILE_TOO_LARGE", "A photo exceeds the size limit")
}

func ErrInvalidMIME() *httpx.AppError {
	return httpx.NewAppError(http.StatusBadRequest, "INVALID_MIME", "Only JPG, PNG, or WebP photos are allowed")
}

func ErrStorageError() *httpx.AppError {
	return httpx.NewAppError(http.StatusInternalServerError, "STORAGE_ERROR", "Failed to store photo")
}

func ErrCaptionTooLong() *httpx.AppError {
	return httpx.NewAppError(http.StatusBadRequest, "CAPTION_TOO_LONG", "Caption must be at most 2000 characters")
}
