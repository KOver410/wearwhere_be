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

type BrandHandler struct{ svc *service.Service }

func NewBrandHandler(svc *service.Service) *BrandHandler { return &BrandHandler{svc: svc} }

func (h *BrandHandler) Me(c *gin.Context) {
	bid := c.MustGet(brandmw.CtxBrandID).(uuid.UUID)
	b, err := h.svc.GetByID(c.Request.Context(), bid)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, gin.H{"brand": domain.ToBrandResponse(b)})
}

func (h *BrandHandler) UpdateMe(c *gin.Context) {
	bid := c.MustGet(brandmw.CtxBrandID).(uuid.UUID)
	var req domain.UpdateBrandRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	if err := h.svc.UpdateOwn(c.Request.Context(), bid, &req); err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	b, err := h.svc.GetByID(c.Request.Context(), bid)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, gin.H{"brand": domain.ToBrandResponse(b)})
}
