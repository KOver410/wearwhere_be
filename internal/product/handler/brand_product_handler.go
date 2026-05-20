package handler

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	brandmw "github.com/wearwhere/wearwhere_be/internal/brand/middleware"
	"github.com/wearwhere/wearwhere_be/internal/product/domain"
	"github.com/wearwhere/wearwhere_be/internal/product/service"
	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

type BrandProductHandler struct{ svc *service.Service }

func NewBrandProductHandler(svc *service.Service) *BrandProductHandler {
	return &BrandProductHandler{svc: svc}
}

func parseIDParam(c *gin.Context, key string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(key))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid "+key)
		return uuid.Nil, false
	}
	return id, true
}

func (h *BrandProductHandler) Create(c *gin.Context) {
	bid := c.MustGet(brandmw.CtxBrandID).(uuid.UUID)
	var req domain.CreateProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	p, err := h.svc.CreateProduct(c.Request.Context(), bid, &req)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.Created(c, gin.H{"product": gin.H{
		"id": p.ID.String(), "slug": p.Slug, "name": p.Name, "status": string(p.Status),
	}})
}

func (h *BrandProductHandler) List(c *gin.Context) {
	bid := c.MustGet(brandmw.CtxBrandID).(uuid.UUID)
	page := paginInt(c, "page", 1, 1, 1_000_000)
	limit := paginInt(c, "limit", 24, 1, 60)
	items, total, err := h.svc.ListOwnProducts(c.Request.Context(), bid, limit, (page-1)*limit)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	out := make([]gin.H, 0, len(items))
	for _, p := range items {
		out = append(out, gin.H{
			"id": p.ID.String(), "slug": p.Slug, "name": p.Name,
			"status": string(p.Status), "currency": p.Currency,
		})
	}
	httpx.OK(c, gin.H{"items": out, "pagination": paginationEnvelope(page, limit, total)})
}

func (h *BrandProductHandler) Get(c *gin.Context) {
	bid := c.MustGet(brandmw.CtxBrandID).(uuid.UUID)
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	p, err := h.svc.GetOwnProduct(c.Request.Context(), id, bid)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, gin.H{"product": gin.H{
		"id": p.ID.String(), "slug": p.Slug, "name": p.Name,
		"description": p.Description, "status": string(p.Status),
		"currency": p.Currency,
	}})
}

func (h *BrandProductHandler) Update(c *gin.Context) {
	bid := c.MustGet(brandmw.CtxBrandID).(uuid.UUID)
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	var req domain.UpdateProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	if err := h.svc.UpdateProduct(c.Request.Context(), id, bid, &req); err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.NoContent(c)
}

func (h *BrandProductHandler) Delete(c *gin.Context) {
	bid := c.MustGet(brandmw.CtxBrandID).(uuid.UUID)
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteProduct(c.Request.Context(), id, bid); err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.NoContent(c)
}

// Variants
func (h *BrandProductHandler) CreateVariant(c *gin.Context) {
	bid := c.MustGet(brandmw.CtxBrandID).(uuid.UUID)
	pid, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	var req domain.CreateVariantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	v, err := h.svc.CreateVariant(c.Request.Context(), pid, bid, &req)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.Created(c, gin.H{"variant": domain.ToVariantResp(v)})
}

func (h *BrandProductHandler) UpdateVariant(c *gin.Context) {
	bid := c.MustGet(brandmw.CtxBrandID).(uuid.UUID)
	pid, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	vid, ok := parseIDParam(c, "variant_id")
	if !ok {
		return
	}
	var req domain.UpdateVariantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	v, err := h.svc.UpdateVariant(c.Request.Context(), vid, pid, bid, &req)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, gin.H{"variant": domain.ToVariantResp(v)})
}

func (h *BrandProductHandler) DeleteVariant(c *gin.Context) {
	bid := c.MustGet(brandmw.CtxBrandID).(uuid.UUID)
	pid, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	vid, ok := parseIDParam(c, "variant_id")
	if !ok {
		return
	}
	if err := h.svc.DeleteVariant(c.Request.Context(), vid, pid, bid); err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.NoContent(c)
}

// Images
func (h *BrandProductHandler) UploadImages(c *gin.Context) {
	bid := c.MustGet(brandmw.CtxBrandID).(uuid.UUID)
	pid, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	form, err := c.MultipartForm()
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	files := form.File["files"]
	if len(files) == 0 {
		files = form.File["files[]"]
	}
	if len(files) == 0 {
		httpx.Error(c, http.StatusBadRequest, "NO_FILES", "No files in request")
		return
	}
	imgs, err := h.svc.UploadImages(c.Request.Context(), pid, bid, files)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	out := make([]domain.ImageResp, 0, len(imgs))
	for _, i := range imgs {
		out = append(out, domain.ToImageResp(i))
	}
	httpx.Created(c, gin.H{"images": out})
}

func (h *BrandProductHandler) UpdateImage(c *gin.Context) {
	bid := c.MustGet(brandmw.CtxBrandID).(uuid.UUID)
	pid, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	iid, ok := parseIDParam(c, "image_id")
	if !ok {
		return
	}
	var req domain.UpdateImageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	img, err := h.svc.UpdateImage(c.Request.Context(), iid, pid, bid, &req)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, gin.H{"image": domain.ToImageResp(img)})
}

func (h *BrandProductHandler) DeleteImage(c *gin.Context) {
	bid := c.MustGet(brandmw.CtxBrandID).(uuid.UUID)
	pid, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	iid, ok := parseIDParam(c, "image_id")
	if !ok {
		return
	}
	if err := h.svc.DeleteImage(c.Request.Context(), iid, pid, bid); err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.NoContent(c)
}

// Pagination helpers shared with catalog handler.
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

func paginationEnvelope(page, limit, total int) gin.H {
	totalPages := (total + limit - 1) / limit
	return gin.H{
		"page": page, "limit": limit, "total": total,
		"total_pages": totalPages, "has_more": page < totalPages,
	}
}
