// Package handler exposes admin HTTP endpoints for managing promo codes.
package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/wearwhere/wearwhere_be/internal/promo/domain"
	"github.com/wearwhere/wearwhere_be/internal/promo/service"
	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

type Handler struct{ svc *service.Service }

func New(s *service.Service) *Handler { return &Handler{svc: s} }

// Create handles POST /admin/promo-codes.
func (h *Handler) Create(c *gin.Context) {
	var req domain.CreatePromoReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "PROMO_INVALID", err.Error())
		return
	}
	p, err := h.svc.CreateCode(c.Request.Context(), req)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.Created(c, domain.ToResp(p))
}

// Update handles PATCH /admin/promo-codes/:id.
func (h *Handler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.ErrorFromApp(c, domain.ErrPromoNotFound)
		return
	}
	var req domain.UpdatePromoReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "PROMO_INVALID", err.Error())
		return
	}
	p, err := h.svc.UpdateCode(c.Request.Context(), id, req)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, domain.ToResp(p))
}

// Get handles GET /admin/promo-codes/:id.
func (h *Handler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.ErrorFromApp(c, domain.ErrPromoNotFound)
		return
	}
	p, err := h.svc.GetCode(c.Request.Context(), id)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, domain.ToResp(p))
}

// List handles GET /admin/promo-codes?page=&page_size=&active_only=.
func (h *Handler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(c.Query("page_size"))
	if pageSize < 1 {
		pageSize = 20
	}
	activeOnly := c.Query("active_only") == "true"

	items, total, err := h.svc.ListCodes(c.Request.Context(), page, pageSize, activeOnly)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	resp := domain.PromoListResp{
		Data:     make([]domain.PromoResp, 0, len(items)),
		Page:     page,
		PageSize: pageSize,
		Total:    total,
	}
	for _, p := range items {
		resp.Data = append(resp.Data, domain.ToResp(p))
	}
	if pageSize > 0 {
		resp.TotalPages = (total + pageSize - 1) / pageSize
	}
	httpx.OK(c, resp)
}
