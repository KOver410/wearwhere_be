package handler

import "github.com/gin-gonic/gin"

func Mount(rg *gin.RouterGroup, h *Handler) {
	rg.GET("/wishlist", h.List)
	rg.GET("/wishlist/contains", h.Contains)
	rg.POST("/wishlist/:product_id", h.Add)
	rg.DELETE("/wishlist/:product_id", h.Remove)
}
