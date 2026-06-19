// Package handler exposes HTTP endpoints for order and checkout operations.
package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	authmw "github.com/wearwhere/wearwhere_be/internal/auth/middleware"
	"github.com/wearwhere/wearwhere_be/internal/order/domain"
	orderrepo "github.com/wearwhere/wearwhere_be/internal/order/repo"
	"github.com/wearwhere/wearwhere_be/internal/order/service"
	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

// Handler holds references to the checkout and order services.
type Handler struct {
	checkout *service.CheckoutService
	order    *service.OrderService
}

// New constructs a Handler from the two order-domain services.
func New(c *service.CheckoutService, o *service.OrderService) *Handler {
	return &Handler{checkout: c, order: o}
}

// PreviewCheckout returns a read-only snapshot of what the order would look like.
// GET /me/checkout/preview?address_id=<uuid>
func (h *Handler) PreviewCheckout(c *gin.Context) {
	userID, ok := authmw.UserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	addressID, err := uuid.Parse(c.Query("address_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing or invalid address_id"})
		return
	}

	resp, err := h.checkout.Preview(c.Request.Context(), userID, addressID, c.Query("promo_code"))
	if err != nil {
		if errors.Is(err, domain.ErrAddressNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "address_not_found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}

// PlaceOrder atomically places an order and returns the order + payment details.
// POST /me/orders
func (h *Handler) PlaceOrder(c *gin.Context) {
	userID, ok := authmw.UserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req domain.PlaceOrderReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "detail": err.Error()})
		return
	}

	resp, pay, err := h.order.PlaceOrder(c.Request.Context(), userID, req)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrCartEmpty):
			c.JSON(http.StatusBadRequest, gin.H{"error": "cart_empty"})
		case errors.Is(err, domain.ErrMinOrderValue):
			c.JSON(http.StatusBadRequest, gin.H{"error": "min_order_value", "min_value_vnd": domain.MinOrderValueVND})
		case errors.Is(err, domain.ErrAddressNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "address_not_found"})
		case errors.Is(err, domain.ErrInsufficientStock):
			c.JSON(http.StatusConflict, gin.H{"error": "insufficient_stock", "detail": err.Error()})
		case errors.Is(err, domain.ErrVariantUnavailable):
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "variant_unavailable", "detail": err.Error()})
		case errors.Is(err, domain.ErrInvalidPaymentMethod):
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_payment_method"})
		case errors.Is(err, domain.ErrPayosLinkCreate):
			c.JSON(http.StatusBadGateway, gin.H{"error": "payos_unavailable", "detail": err.Error()})
		default:
			// Promo-code errors (and any other AppError) carry their own
			// HTTP status + stable code (PROMO_EXPIRED, PROMO_ALREADY_USED, ...).
			var appErr *httpx.AppError
			if errors.As(err, &appErr) {
				c.JSON(appErr.Status, gin.H{"error": appErr.Code, "message": appErr.Message})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusCreated, gin.H{"order": resp, "payment": pay})
}

// ListOrders returns a paginated list of the authenticated user's orders.
// GET /me/orders
func (h *Handler) ListOrders(c *gin.Context) {
	userID, ok := authmw.UserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	filter := orderrepo.ListFilter{UserID: userID, Page: 1, PageSize: 20}

	if statusStr := c.Query("status"); statusStr != "" {
		for _, s := range strings.Split(statusStr, ",") {
			filter.Statuses = append(filter.Statuses, domain.OrderStatus(strings.TrimSpace(s)))
		}
	}
	if p, _ := strconv.Atoi(c.Query("page")); p > 0 {
		filter.Page = p
	}
	if ps, _ := strconv.Atoi(c.Query("page_size")); ps > 0 {
		filter.PageSize = ps
	}
	if from := c.Query("from"); from != "" {
		filter.FromTime = &from
	}
	if to := c.Query("to"); to != "" {
		filter.ToTime = &to
	}

	resp, err := h.order.List(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}

// DetailOrder returns the full order detail for the authenticated user.
// GET /me/orders/:order_no
func (h *Handler) DetailOrder(c *gin.Context) {
	userID, ok := authmw.UserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	orderNo := c.Param("order_no")
	resp, err := h.order.Detail(c.Request.Context(), userID, orderNo)
	if err != nil {
		if errors.Is(err, domain.ErrOrderNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "order_not_found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}

// CancelOrder cancels an order owned by the authenticated user.
// POST /me/orders/:order_no/cancel
func (h *Handler) CancelOrder(c *gin.Context) {
	userID, ok := authmw.UserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	orderNo := c.Param("order_no")
	var req domain.CancelOrderReq
	_ = c.ShouldBindJSON(&req)

	resp, err := h.order.Cancel(c.Request.Context(), userID, orderNo, req.Reason)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrOrderNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "order_not_found"})
		case errors.Is(err, domain.ErrCancelPaidNotSupported):
			c.JSON(http.StatusConflict, gin.H{"error": "cancel_not_allowed", "subcode": "paid_not_supported"})
		case errors.Is(err, domain.ErrCancelNotAllowed):
			subcode := "already_shipped"
			msg := err.Error()
			for _, code := range []string{"already_shipped", "already_cancelled", "already_completed"} {
				if strings.Contains(msg, code) {
					subcode = code
					break
				}
			}
			c.JSON(http.StatusConflict, gin.H{"error": "cancel_not_allowed", "subcode": subcode})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, resp)
}
