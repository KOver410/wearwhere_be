package handler

import (
	"fmt"
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

// ── public brand endpoints ──────────────────────────────────────────────────

type BrandsPublicHandler struct{ svc *service.Service }

func NewBrandsPublicHandler(svc *service.Service) *BrandsPublicHandler {
	return &BrandsPublicHandler{svc: svc}
}

func (h *BrandsPublicHandler) List(c *gin.Context) {
	q := c.Query("q")
	sort := c.Query("sort")
	page := paginInt(c, "page", 1, 1, 1_000_000)
	limit := paginInt(c, "limit", 24, 1, 60)
	items, total, err := h.svc.ListBrands(c.Request.Context(), q, sort, limit, (page-1)*limit)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	out := make([]domain.BrandResponse, 0, len(items))
	for _, b := range items {
		out = append(out, domain.ToBrandResponse(b))
	}
	httpx.OK(c, gin.H{
		"items":      out,
		"pagination": paginEnvelope(page, limit, total),
	})
}

func (h *BrandsPublicHandler) Detail(c *gin.Context) {
	slug := c.Param("slug")
	b, err := h.svc.GetBySlug(c.Request.Context(), slug)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	addrs, err := h.svc.ListAddresses(c.Request.Context(), b.ID, false) // public only
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	addrOut := make([]domain.AddressResponse, 0, len(addrs))
	for _, a := range addrs {
		addrOut = append(addrOut, domain.ToAddressResponse(a))
	}
	httpx.OK(c, gin.H{
		"brand":     domain.ToBrandResponse(b),
		"addresses": addrOut,
	})
}

func paginInt(c *gin.Context, key string, def, min, max int) int {
	v := c.Query(key)
	if v == "" {
		return def
	}
	var n int
	_, err := fmt.Sscanf(v, "%d", &n)
	if err != nil || n < min {
		return def
	}
	if n > max {
		return max
	}
	return n
}

func paginEnvelope(page, limit, total int) gin.H {
	totalPages := (total + limit - 1) / limit
	if totalPages == 0 {
		totalPages = 1
	}
	return gin.H{
		"page": page, "limit": limit, "total": total,
		"total_pages": totalPages, "has_more": page < totalPages,
	}
}
