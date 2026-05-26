package handler

import "github.com/gin-gonic/gin"

// MountPublic registers the PayOS webhook endpoint on the given router group.
// Typically mounted at the root (no auth required — PayOS calls this directly).
func MountPublic(rg *gin.RouterGroup, h *Handler) {
	rg.POST("/payments/payos/webhook", h.PayosWebhook)
}

// MountDev registers dev-only mock/simulate endpoints.
// Only call this when the server is running in mock/dev mode.
func MountDev(r *gin.RouterGroup, h *Handler) {
	r.GET("/payos/mock-checkout", h.MockCheckoutPage)
	r.POST("/payos/simulate", h.SimulateWebhook)
}
