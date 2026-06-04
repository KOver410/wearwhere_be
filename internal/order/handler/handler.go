// Package handler exposes HTTP endpoints for order and checkout operations.
package handler

import (
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
		httpx.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
		return
	}

	addressID, err := uuid.Parse(c.Query("address_id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, "INVALID_ADDRESS_ID", "Missing or invalid address_id")
		return
	}

	resp, err := h.checkout.Preview(c.Request.Context(), userID, addressID)
	if err != nil {
		writeOrderError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

// PlaceOrder atomically places an order and returns the order + payment details.
// POST /me/orders
func (h *Handler) PlaceOrder(c *gin.Context) {
	userID, ok := authmw.UserID(c)
	if !ok {
		httpx.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
		return
	}

	var req domain.PlaceOrderReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "INVALID_BODY", "Invalid request body")
		return
	}

	resp, pay, err := h.order.PlaceOrder(c.Request.Context(), userID, req)
	if err != nil {
		writeOrderError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"order": resp, "payment": pay})
}

// ListOrders returns a paginated list of the authenticated user's orders.
// GET /me/orders
func (h *Handler) ListOrders(c *gin.Context) {
	userID, ok := authmw.UserID(c)
	if !ok {
		httpx.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
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
		writeOrderError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

// DetailOrder returns the full order detail for the authenticated user.
// GET /me/orders/:order_no
func (h *Handler) DetailOrder(c *gin.Context) {
	userID, ok := authmw.UserID(c)
	if !ok {
		httpx.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
		return
	}

	orderNo := c.Param("order_no")
	resp, err := h.order.Detail(c.Request.Context(), userID, orderNo)
	if err != nil {
		writeOrderError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

// CancelOrder cancels an order owned by the authenticated user.
// POST /me/orders/:order_no/cancel
func (h *Handler) CancelOrder(c *gin.Context) {
	userID, ok := authmw.UserID(c)
	if !ok {
		httpx.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
		return
	}

	orderNo := c.Param("order_no")
	var req domain.CancelOrderReq
	_ = c.ShouldBindJSON(&req)

	resp, err := h.order.Cancel(c.Request.Context(), userID, orderNo, req.Reason)
	if err != nil {
		writeOrderError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}
