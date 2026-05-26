package domain

import (
	"net/http"

	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

// Auth-related AppErrors. Handlers translate these to HTTP responses
// uniformly via httpx.ErrorFromApp.
var (
	ErrInvalidCredentials   = httpx.NewAppError(http.StatusUnauthorized, "INVALID_CREDENTIALS", "Email/phone or password is incorrect")
	ErrUnauthorized         = httpx.NewAppError(http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
	ErrForbidden            = httpx.NewAppError(http.StatusForbidden, "FORBIDDEN", "You don't have permission to perform this action")
	ErrAccountLocked        = httpx.NewAppError(http.StatusForbidden, "ACCOUNT_LOCKED", "Account temporarily locked due to too many failed attempts")
	ErrAccountDeleted       = httpx.NewAppError(http.StatusForbidden, "ACCOUNT_DELETED", "This account has been deleted")
	ErrEmailExists          = httpx.NewAppError(http.StatusConflict, "EMAIL_EXISTS", "Email already registered")
	ErrPhoneExists          = httpx.NewAppError(http.StatusConflict, "PHONE_EXISTS", "Phone already registered")
	ErrEmailOrPhoneRequired = httpx.NewAppError(http.StatusBadRequest, "EMAIL_OR_PHONE_REQUIRED", "Either email or phone is required")
	ErrUserNotFound         = httpx.NewAppError(http.StatusNotFound, "USER_NOT_FOUND", "User not found")
	ErrInvalidOTP           = httpx.NewAppError(http.StatusBadRequest, "INVALID_OTP", "OTP is invalid or expired")
	ErrTooManyOTPRequests   = httpx.NewAppError(http.StatusTooManyRequests, "OTP_RATE_LIMITED", "Too many OTP requests. Try again later")
	ErrInvalidRefresh       = httpx.NewAppError(http.StatusUnauthorized, "INVALID_REFRESH_TOKEN", "Refresh token is invalid, revoked, or expired")
	ErrPasswordReuse        = httpx.NewAppError(http.StatusBadRequest, "PASSWORD_REUSED", "New password must differ from your last 3 passwords")
	ErrSamePassword         = httpx.NewAppError(http.StatusBadRequest, "SAME_PASSWORD", "New password must differ from current password")
	ErrSocialTokenInvalid   = httpx.NewAppError(http.StatusUnauthorized, "SOCIAL_TOKEN_INVALID", "Social provider token is invalid")
	ErrPendingOrders        = httpx.NewAppError(http.StatusConflict, "PENDING_ORDERS", "Cannot delete account with pending orders")
	ErrRateLimited          = httpx.NewAppError(http.StatusTooManyRequests, "RATE_LIMITED", "Too many requests")
	ErrInternal             = httpx.NewAppError(http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
)
