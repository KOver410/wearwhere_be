package domain

import (
	"net/http"

	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

// ErrPromoNotFound is returned for unknown OR inactive codes — the two collapse
// so the API does not leak which codes exist.
var ErrPromoNotFound = httpx.NewAppError(
	http.StatusNotFound, "PROMO_NOT_FOUND", "Mã giảm giá không tồn tại hoặc đã ngừng áp dụng")

// ErrPromoNotStarted is returned when now < starts_at.
var ErrPromoNotStarted = httpx.NewAppError(
	http.StatusUnprocessableEntity, "PROMO_NOT_STARTED", "Mã giảm giá chưa đến thời gian áp dụng")

// ErrPromoExpired is returned when now > ends_at.
var ErrPromoExpired = httpx.NewAppError(
	http.StatusUnprocessableEntity, "PROMO_EXPIRED", "Mã giảm giá đã hết hạn")

// ErrPromoMinOrder is returned when the subtotal is below the code's minimum.
var ErrPromoMinOrder = httpx.NewAppError(
	http.StatusUnprocessableEntity, "PROMO_MIN_ORDER", "Đơn hàng chưa đạt giá trị tối thiểu để áp dụng mã")

// ErrPromoAlreadyUsed is returned when the user has already redeemed the code.
var ErrPromoAlreadyUsed = httpx.NewAppError(
	http.StatusConflict, "PROMO_ALREADY_USED", "Bạn đã sử dụng mã giảm giá này rồi")

// ErrPromoNotApplicable is returned when the computed discount is zero.
var ErrPromoNotApplicable = httpx.NewAppError(
	http.StatusUnprocessableEntity, "PROMO_NOT_APPLICABLE", "Mã giảm giá không áp dụng được cho đơn hàng này")

// ErrPromoCodeExists is returned by admin create on a duplicate code.
var ErrPromoCodeExists = httpx.NewAppError(
	http.StatusConflict, "PROMO_CODE_EXISTS", "Mã giảm giá đã tồn tại")

// ErrInvalidPromo is returned by admin create/update on bad input.
var ErrInvalidPromo = httpx.NewAppError(
	http.StatusBadRequest, "PROMO_INVALID", "Dữ liệu mã giảm giá không hợp lệ")
