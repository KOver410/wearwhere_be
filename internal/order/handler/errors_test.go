package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/order/domain"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// decodeError runs writeOrderError against err and returns the recorded HTTP
// status and the parsed error envelope.
func decodeError(t *testing.T, err error) (int, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	writeOrderError(c, err)

	var body struct {
		Error map[string]any `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body), "body: %s", rec.Body.String())
	require.NotNil(t, body.Error, "response must use the shared error envelope")
	return rec.Code, body.Error
}

func TestWriteOrderError_StatusAndCode(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantHTTP int
		wantCode string
	}{
		{"cart empty", domain.ErrCartEmpty, 400, "CART_EMPTY"},
		{"min order value", domain.ErrMinOrderValue, 400, "MIN_ORDER_VALUE"},
		{"address not found", domain.ErrAddressNotFound, 404, "ADDRESS_NOT_FOUND"},
		{"insufficient stock", domain.ErrInsufficientStock, 409, "INSUFFICIENT_STOCK"},
		{"variant unavailable", domain.ErrVariantUnavailable, 422, "VARIANT_UNAVAILABLE"},
		{"invalid payment method", domain.ErrInvalidPaymentMethod, 400, "INVALID_PAYMENT_METHOD"},
		{"payos link create", domain.ErrPayosLinkCreate, 502, "PAYOS_UNAVAILABLE"},
		{"order not found", domain.ErrOrderNotFound, 404, "ORDER_NOT_FOUND"},
		{"cancel not allowed", domain.ErrCancelNotAllowed, 409, "CANCEL_NOT_ALLOWED"},
		{"cancel paid not supported", domain.ErrCancelPaidNotSupported, 409, "CANCEL_NOT_ALLOWED"},
		{"idor", domain.ErrIDOR, 403, "FORBIDDEN"},
		{"unknown", errors.New("boom"), 500, "INTERNAL_ERROR"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, env := decodeError(t, tc.err)
			assert.Equal(t, tc.wantHTTP, status)
			assert.Equal(t, tc.wantCode, env["code"])
			assert.NotEmpty(t, env["message"], "message must not be empty")
		})
	}
}

func TestWriteOrderError_Details(t *testing.T) {
	t.Run("min order value exposes min_value_vnd", func(t *testing.T) {
		_, env := decodeError(t, domain.ErrMinOrderValue)
		details, ok := env["details"].(map[string]any)
		require.True(t, ok, "min order value must include details")
		// JSON numbers decode to float64.
		assert.Equal(t, float64(domain.MinOrderValueVND), details["min_value_vnd"])
	})

	t.Run("insufficient stock exposes refresh reason", func(t *testing.T) {
		_, env := decodeError(t, domain.ErrInsufficientStock)
		details, ok := env["details"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "Refresh cart and retry", details["reason"])
	})

	t.Run("variant unavailable exposes refresh reason", func(t *testing.T) {
		_, env := decodeError(t, domain.ErrVariantUnavailable)
		details, ok := env["details"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "Refresh cart and retry", details["reason"])
	})

	t.Run("cancel not allowed subcode", func(t *testing.T) {
		_, env := decodeError(t, domain.ErrCancelNotAllowed)
		details, ok := env["details"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "not_allowed", details["subcode"])
	})

	t.Run("cancel paid not supported subcode", func(t *testing.T) {
		_, env := decodeError(t, domain.ErrCancelPaidNotSupported)
		details, ok := env["details"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "paid_not_supported", details["subcode"])
	})
}

func TestWriteOrderError_NoDetailsForSimpleCases(t *testing.T) {
	for _, err := range []error{
		domain.ErrCartEmpty,
		domain.ErrAddressNotFound,
		domain.ErrInvalidPaymentMethod,
		domain.ErrPayosLinkCreate,
		domain.ErrOrderNotFound,
	} {
		err := err
		t.Run(err.Error(), func(t *testing.T) {
			_, env := decodeError(t, err)
			_, hasDetails := env["details"]
			assert.False(t, hasDetails, "%v should not carry details", err)
		})
	}
}

func TestWriteOrderError_DoesNotLeakInternalError(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	writeOrderError(c, fmt.Errorf("wrapped: %w", errors.New("boom secret detail")))

	assert.Equal(t, 500, rec.Code)
	assert.False(t, strings.Contains(rec.Body.String(), "boom"),
		"internal response must not leak err.Error(); body=%s", rec.Body.String())
}

func TestWriteOrderError_MatchesWrappedSentinels(t *testing.T) {
	wrapped := fmt.Errorf("service failed: %w", domain.ErrInsufficientStock)
	status, env := decodeError(t, wrapped)
	assert.Equal(t, 409, status)
	assert.Equal(t, "INSUFFICIENT_STOCK", env["code"])
}
