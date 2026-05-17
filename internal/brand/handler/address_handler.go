package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/wearwhere/wearwhere_be/internal/brand/domain"
	brandmw "github.com/wearwhere/wearwhere_be/internal/brand/middleware"
	"github.com/wearwhere/wearwhere_be/internal/brand/service"
	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

type AddressHandler struct{ svc *service.Service }

func NewAddressHandler(svc *service.Service) *AddressHandler { return &AddressHandler{svc: svc} }

func (h *AddressHandler) List(c *gin.Context) {
	bid := c.MustGet(brandmw.CtxBrandID).(uuid.UUID)
	items, err := h.svc.ListAddresses(c.Request.Context(), bid, true)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	resp := make([]domain.AddressResponse, 0, len(items))
	for _, a := range items {
		resp = append(resp, domain.ToAddressResponse(a))
	}
	httpx.OK(c, gin.H{"items": resp})
}

func (h *AddressHandler) Create(c *gin.Context) {
	bid := c.MustGet(brandmw.CtxBrandID).(uuid.UUID)
	var req domain.CreateAddressRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	a, err := h.svc.CreateAddress(c.Request.Context(), bid, &req)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.Created(c, gin.H{"address": domain.ToAddressResponse(a)})
}

func (h *AddressHandler) Update(c *gin.Context) {
	bid := c.MustGet(brandmw.CtxBrandID).(uuid.UUID)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid address id")
		return
	}
	var req domain.UpdateAddressRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	a, err := h.svc.UpdateAddress(c.Request.Context(), id, bid, &req)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, gin.H{"address": domain.ToAddressResponse(a)})
}

func (h *AddressHandler) Delete(c *gin.Context) {
	bid := c.MustGet(brandmw.CtxBrandID).(uuid.UUID)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid address id")
		return
	}
	if err := h.svc.DeleteAddress(c.Request.Context(), id, bid); err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.NoContent(c)
}
