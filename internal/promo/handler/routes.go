package handler

import "github.com/gin-gonic/gin"

// MountAdmin registers admin promo-code management routes. Caller chains admin
// auth onto rg.
func MountAdmin(rg *gin.RouterGroup, h *Handler) {
	g := rg.Group("/promo-codes")
	g.POST("", h.Create)
	g.GET("", h.List)
	g.GET("/:id", h.Get)
	g.PATCH("/:id", h.Update)
}
