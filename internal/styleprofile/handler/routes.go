package handler

import "github.com/gin-gonic/gin"

// Mount registers style-profile routes under a group that already applies
// RequireAuth + RequireRole(customer) (the /me customer group).
func Mount(rg *gin.RouterGroup, h *Handler) {
	rg.GET("/style-profile", h.Get)
	rg.PUT("/style-profile", h.Put)
}
