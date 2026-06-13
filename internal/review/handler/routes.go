package handler

import "github.com/gin-gonic/gin"

// MountReviewsPublic registers the public read route (no auth).
func MountReviewsPublic(rg *gin.RouterGroup, h *Handler) {
	rg.GET("/products/:id/reviews", h.List)
}

// MountReviewsAuthed registers customer-authed write routes. The caller must
// have chained RequireAuth onto rg.
func MountReviewsAuthed(rg *gin.RouterGroup, h *Handler) {
	rg.POST("/products/:id/reviews", h.Create)
	rg.PATCH("/reviews/:id", h.Update)
	rg.DELETE("/reviews/:id", h.Delete)
}
