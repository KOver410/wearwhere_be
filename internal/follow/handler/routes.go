package handler

import "github.com/gin-gonic/gin"

// MountFollowAuthed registers customer-authed follow routes. Caller chains RequireAuth.
func MountFollowAuthed(rg *gin.RouterGroup, h *Handler) {
	rg.POST("/users/:id/follow", h.FollowUser)
	rg.DELETE("/users/:id/follow", h.UnfollowUser)
	rg.POST("/brand-follows/:id", h.FollowBrand)
	rg.DELETE("/brand-follows/:id", h.UnfollowBrand)
	rg.GET("/me/following/users", h.FollowingUsers)
	rg.GET("/me/following/brands", h.FollowingBrands)
}
