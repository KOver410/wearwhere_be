// Package handler exposes the admin HTTP endpoint for listing users.
package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/wearwhere/wearwhere_be/internal/admin/user/domain"
	"github.com/wearwhere/wearwhere_be/internal/admin/user/service"
	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

type Handler struct{ svc *service.Service }

func New(s *service.Service) *Handler { return &Handler{svc: s} }

// List handles GET /admin/users?q=&sort=&order=&page=&page_size=.
// Out-of-range / unknown values are normalized by the service.
func (h *Handler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	pageSize, _ := strconv.Atoi(c.Query("page_size"))

	f := domain.ListUsersFilter{
		Q:        c.Query("q"),
		Sort:     c.Query("sort"),
		Order:    c.Query("order"),
		Page:     page,
		PageSize: pageSize,
	}
	resp, err := h.svc.ListUsers(c.Request.Context(), f)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, resp)
}
