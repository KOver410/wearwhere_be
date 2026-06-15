// Package handler exposes follow HTTP endpoints (all customer-authed).
package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	authmw "github.com/wearwhere/wearwhere_be/internal/auth/middleware"
	"github.com/wearwhere/wearwhere_be/internal/follow/domain"
	"github.com/wearwhere/wearwhere_be/internal/follow/service"
	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

type Handler struct{ svc *service.Service }

func New(s *service.Service) *Handler { return &Handler{svc: s} }

func (h *Handler) userID(c *gin.Context) uuid.UUID { id, _ := authmw.UserID(c); return id }

func parsePage(c *gin.Context) (page, limit int) {
	page, _ = strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ = strconv.Atoi(c.DefaultQuery("limit", "20"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 20
	}
	return
}

func (h *Handler) FollowUser(c *gin.Context) {
	target, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.ErrorFromApp(c, domain.ErrUserNotFound())
		return
	}
	resp, err := h.svc.FollowUser(c.Request.Context(), h.userID(c), target)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, resp)
}

func (h *Handler) UnfollowUser(c *gin.Context) {
	target, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.ErrorFromApp(c, domain.ErrUserNotFound())
		return
	}
	resp, err := h.svc.UnfollowUser(c.Request.Context(), h.userID(c), target)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, resp)
}

func (h *Handler) FollowBrand(c *gin.Context) {
	target, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.ErrorFromApp(c, domain.ErrBrandNotFound())
		return
	}
	resp, err := h.svc.FollowBrand(c.Request.Context(), h.userID(c), target)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, resp)
}

func (h *Handler) UnfollowBrand(c *gin.Context) {
	target, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.ErrorFromApp(c, domain.ErrBrandNotFound())
		return
	}
	resp, err := h.svc.UnfollowBrand(c.Request.Context(), h.userID(c), target)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, resp)
}

func (h *Handler) FollowingUsers(c *gin.Context) {
	page, limit := parsePage(c)
	items, total, err := h.svc.ListFollowingUsers(c.Request.Context(), h.userID(c), page, limit)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	if items == nil {
		items = []domain.FollowingUserItem{}
	}
	httpx.OK(c, gin.H{"items": items, "pagination": domain.NewPagination(page, limit, total)})
}

func (h *Handler) FollowingBrands(c *gin.Context) {
	page, limit := parsePage(c)
	items, total, err := h.svc.ListFollowingBrands(c.Request.Context(), h.userID(c), page, limit)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	if items == nil {
		items = []domain.FollowingBrandItem{}
	}
	httpx.OK(c, gin.H{"items": items, "pagination": domain.NewPagination(page, limit, total)})
}

