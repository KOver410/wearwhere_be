package handler

import "github.com/gin-gonic/gin"

// MountStoresPublic registers public read-only store discovery routes (no auth).
func MountStoresPublic(rg *gin.RouterGroup, h *Handler) {
	rg.GET("/stores/nearby", h.Nearby)
	rg.GET("/stores", h.Search)
	rg.GET("/stores/:id", h.Detail)
	rg.GET("/stores/:id/directions", h.Directions)
}
