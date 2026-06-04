package handler

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/wearwhere/wearwhere_be/internal/order/service"
	"github.com/wearwhere/wearwhere_be/internal/shipping/goship"
)

type ShippingWebhookHandler struct {
	svc      *service.ShippingWebhookService
	goship   goship.Shipper
	mockMode bool
}

func NewShippingWebhookHandler(s *service.ShippingWebhookService, gs goship.Shipper, mockMode bool) *ShippingWebhookHandler {
	return &ShippingWebhookHandler{svc: s, goship: gs, mockMode: mockMode}
}

// GoshipWebhook — POST /shipping/goship/webhook
func (h *ShippingWebhookHandler) GoshipWebhook(c *gin.Context) {
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "read_body"})
		return
	}
	if !h.mockMode {
		sig := c.GetHeader("x-goship-hmac-sha256")
		if err := h.goship.VerifyWebhookSignature(raw, sig); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_signature"})
			return
		}
	}
	var p goship.WebhookPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_payload"})
		return
	}
	if err := h.svc.HandleGoshipWebhook(c.Request.Context(), p); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"received": true})
}

// SimulateWebhook — POST /dev/goship/simulate (dev only)
func (h *ShippingWebhookHandler) SimulateWebhook(c *gin.Context) {
	var req struct {
		TrackingNo string `json:"tracking_no" form:"tracking_no"`
		Status     string `json:"status" form:"status"`
		IsReturn   int    `json:"is_return" form:"is_return"`
		IsLost     int    `json:"is_lost" form:"is_lost"`
	}
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
		return
	}
	p := goship.WebhookPayload{Code: req.TrackingNo, StatusText: req.Status, IsReturn: req.IsReturn, IsLost: req.IsLost}
	if err := h.svc.HandleGoshipWebhook(c.Request.Context(), p); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"simulated": true})
}
