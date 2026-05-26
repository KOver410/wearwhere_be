// Package handler exposes HTTP endpoints for PayOS payment webhooks and dev helpers.
package handler

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/wearwhere/wearwhere_be/internal/payment/payos"
	"github.com/wearwhere/wearwhere_be/internal/payment/service"
)

// Handler holds the webhook service, a payos client (for signature verification),
// and a flag indicating whether dev/mock endpoints are enabled.
type Handler struct {
	webhook  *service.WebhookService
	payos    payos.Client
	mockMode bool
}

// New constructs a Handler.
func New(w *service.WebhookService, pc payos.Client, mockMode bool) *Handler {
	return &Handler{webhook: w, payos: pc, mockMode: mockMode}
}

// PayosWebhook receives a payment confirmation callback from PayOS.
// POST /payments/payos/webhook
func (h *Handler) PayosWebhook(c *gin.Context) {
	var p payos.WebhookPayload
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_payload"})
		return
	}

	// Verify HMAC signature unless running in mock mode.
	if !h.mockMode {
		if err := h.payos.VerifyWebhookSignature(p); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_signature"})
			return
		}
	}

	if err := h.webhook.HandlePayosWebhook(c.Request.Context(), p); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"received": true})
}

// MockCheckoutPage renders a simple HTML form that simulates the PayOS payment
// page. Only available in dev/mock mode.
// GET /dev/payos/mock-checkout?orderCode=<code>
func (h *Handler) MockCheckoutPage(c *gin.Context) {
	orderCode := c.Query("orderCode")
	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><title>Mock PayOS Checkout</title></head>
<body>
<h2>Mock PayOS — orderCode: %s</h2>
<form method="POST" action="/dev/payos/simulate">
  <input type="hidden" name="orderCode" value="%s"/>
  <button name="action" value="success">Pay (success)</button>
  <button name="action" value="cancel">Cancel</button>
</form>
</body>
</html>`, orderCode, orderCode)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}

// SimulateWebhook constructs a mock PayOS webhook payload and dispatches it
// to the webhook service. Only available in dev/mock mode.
// POST /dev/payos/simulate
func (h *Handler) SimulateWebhook(c *gin.Context) {
	var req struct {
		OrderCode int64  `json:"orderCode" form:"orderCode"`
		Action    string `json:"action" form:"action"` // "success" or "cancel"
	}
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
		return
	}

	success := req.Action == "success"
	code := "00"
	desc := "success"
	if !success {
		code = "01"
		desc = "cancelled"
	}

	payload := payos.WebhookPayload{
		Code:    code,
		Desc:    desc,
		Success: success,
		Data: payos.WebhookData{
			OrderCode: req.OrderCode,
			Code:      code,
			Desc:      desc,
		},
		Signature: "mock",
	}

	if err := h.webhook.HandlePayosWebhook(c.Request.Context(), payload); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"simulated": true, "success": success, "orderCode": req.OrderCode})
}
