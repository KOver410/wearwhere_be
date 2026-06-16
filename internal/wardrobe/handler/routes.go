package handler

import "github.com/gin-gonic/gin"

// Mount registers wardrobe routes under the /me customer group
// (RequireAuth + RequireRole(customer) already applied).
func Mount(rg *gin.RouterGroup, h *Handler) {
	rg.GET("/wardrobe", h.Get)
	rg.POST("/wardrobe/regenerate", h.Regenerate)
}
