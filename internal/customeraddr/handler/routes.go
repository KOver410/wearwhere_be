package handler

import "github.com/gin-gonic/gin"

// Mount registers customer-address routes onto a group that already has
// RequireAuth + RequireRole(customer) applied (e.g., /me).
func Mount(rg *gin.RouterGroup, h *Handler) {
	rg.GET("/addresses", h.List)
	rg.POST("/addresses", h.Create)
	rg.GET("/addresses/:id", h.Get)
	rg.PATCH("/addresses/:id", h.Update)
	rg.DELETE("/addresses/:id", h.Delete)
}
