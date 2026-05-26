// Package handler exposes HTTP endpoints for customer addresses.
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	authmw "github.com/wearwhere/wearwhere_be/internal/auth/middleware"
	"github.com/wearwhere/wearwhere_be/internal/customeraddr/domain"
	"github.com/wearwhere/wearwhere_be/internal/customeraddr/service"
	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

type Handler struct {
	svc *service.CustomerAddressService
}

func New(s *service.CustomerAddressService) *Handler { return &Handler{svc: s} }

func (h *Handler) userID(c *gin.Context) uuid.UUID {
	id, _ := authmw.UserID(c)
	return id
}

func (h *Handler) List(c *gin.Context) {
	items, err := h.svc.List(c.Request.Context(), h.userID(c))
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	out := make([]domain.AddressResponse, 0, len(items))
	for _, a := range items {
		out = append(out, domain.ToAddressResponse(a))
	}
	httpx.OK(c, gin.H{"items": out})
}

func (h *Handler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.ErrorFromApp(c, domain.ErrAddressNotFound)
		return
	}
	a, err := h.svc.Get(c.Request.Context(), id, h.userID(c))
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, domain.ToAddressResponse(a))
}

func (h *Handler) Create(c *gin.Context) {
	var req domain.CreateAddressRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	a, err := h.svc.Create(c.Request.Context(), h.userID(c), &req)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.Created(c, domain.ToAddressResponse(a))
}

func (h *Handler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.ErrorFromApp(c, domain.ErrAddressNotFound)
		return
	}
	var req domain.UpdateAddressRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	a, err := h.svc.Update(c.Request.Context(), id, h.userID(c), &req)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, domain.ToAddressResponse(a))
}

func (h *Handler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.ErrorFromApp(c, domain.ErrAddressNotFound)
		return
	}
	if err := h.svc.Delete(c.Request.Context(), id, h.userID(c)); err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.NoContent(c)
}
