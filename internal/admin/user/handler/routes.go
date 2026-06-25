package handler

import "github.com/gin-gonic/gin"

// MountAdmin registers admin user-management routes. The caller chains admin
// auth (RequireAuth + RequireRole(admin)) onto rg.
func MountAdmin(rg *gin.RouterGroup, h *Handler) {
	g := rg.Group("/users")
	g.GET("", h.List)
}
