package handler

import "github.com/gin-gonic/gin"

func Mount(rg *gin.RouterGroup, h *Handler) {
	rg.GET("/cart", h.Get)
	rg.POST("/cart/items", h.Add)
	rg.PATCH("/cart/items/:item_id", h.Update)
	rg.DELETE("/cart/items/:item_id", h.Delete)
	rg.DELETE("/cart", h.Clear)
}
