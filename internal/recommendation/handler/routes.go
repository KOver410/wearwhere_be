package handler

import "github.com/gin-gonic/gin"

// Mount registers the recommendation route under a group that already applies
// RequireAuth + RequireRole(customer) (the /me customer group).
func Mount(rg *gin.RouterGroup, h *Handler) {
	rg.GET("/recommendations", h.List)
}
