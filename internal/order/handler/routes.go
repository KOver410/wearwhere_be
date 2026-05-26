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
