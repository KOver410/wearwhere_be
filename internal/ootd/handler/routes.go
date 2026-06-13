package handler

import "github.com/gin-gonic/gin"

// MountOOTDPublic registers public read routes (no auth). Note: Feed/Detail/ByUser
// read the optional authed user id for liked_by_me, so if the caller wants
// liked_by_me populated for logged-in users it may chain an optional-auth middleware;
// without it, liked_by_me is always false (acceptable).
func MountOOTDPublic(rg *gin.RouterGroup, h *Handler) {
	rg.GET("/ootd", h.Feed)
	rg.GET("/ootd/:id", h.Detail)
	rg.GET("/ootd/:id/comments", h.ListComments)
	rg.GET("/users/:id/ootd", h.ByUser)
}

// MountOOTDAuthed registers customer-authed write routes. Caller chains RequireAuth.
func MountOOTDAuthed(rg *gin.RouterGroup, h *Handler) {
	rg.POST("/ootd", h.Create)
	rg.PATCH("/ootd/:id", h.UpdateCaption)
	rg.DELETE("/ootd/:id", h.Delete)
	rg.POST("/ootd/:id/like", h.Like)
	rg.DELETE("/ootd/:id/like", h.Unlike)
	rg.POST("/ootd/:id/comments", h.AddComment)
	rg.DELETE("/ootd-comments/:id", h.DeleteComment)
}
