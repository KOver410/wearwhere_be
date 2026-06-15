package handler

import "github.com/gin-gonic/gin"

// MountBlockAuthed registers customer-authed block routes. Caller chains RequireAuth.
func MountBlockAuthed(rg *gin.RouterGroup, h *Handler) {
	rg.POST("/users/:id/block", h.BlockUser)
	rg.DELETE("/users/:id/block", h.UnblockUser)
	rg.GET("/me/blocks", h.ListBlocked)
}
