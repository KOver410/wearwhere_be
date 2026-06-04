package handler

import "github.com/gin-gonic/gin"

// Mount registers customer-facing order and checkout endpoints under /me/...
func Mount(rg *gin.RouterGroup, h *Handler) {
	rg.GET("/checkout/preview", h.PreviewCheckout)
	rg.POST("/orders", h.PlaceOrder)
	rg.GET("/orders", h.ListOrders)
	rg.GET("/orders/:order_no", h.DetailOrder)
	rg.POST("/orders/:order_no/cancel", h.CancelOrder)
}

// MountBrand registers brand fulfillment routes. Caller chains brand auth onto rg.
func MountBrand(rg *gin.RouterGroup, h *BrandFulfillmentHandler) {
	o := rg.Group("/orders")
	o.GET("", h.List)
	o.GET("/:sub_order_id", h.Detail)
	o.POST("/:sub_order_id/confirm", h.Confirm)
	o.POST("/:sub_order_id/ship", h.Ship)
}

// MountShippingPublic registers the Goship status webhook (no auth).
func MountShippingPublic(rg *gin.RouterGroup, h *ShippingWebhookHandler) {
	rg.POST("/shipping/goship/webhook", h.GoshipWebhook)
}

// MountShippingDev registers dev-only simulate endpoint.
func MountShippingDev(rg *gin.RouterGroup, h *ShippingWebhookHandler) {
	rg.POST("/goship/simulate", h.SimulateWebhook)
}
