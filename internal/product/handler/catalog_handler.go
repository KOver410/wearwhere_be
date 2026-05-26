package handler

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	brandrepo "github.com/wearwhere/wearwhere_be/internal/brand/repo"
	"github.com/wearwhere/wearwhere_be/internal/product/domain"
	productrepo "github.com/wearwhere/wearwhere_be/internal/product/repo"
	"github.com/wearwhere/wearwhere_be/internal/product/service"
	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

type CatalogHandler struct {
	svc       *service.CatalogService
	categoryR productrepo.CategoryRepo
	styleR    productrepo.StyleTagRepo
	brandR    brandrepo.BrandRepo
}

func NewCatalogHandler(
	svc *service.CatalogService,
	cr productrepo.CategoryRepo,
	sr productrepo.StyleTagRepo,
	br brandrepo.BrandRepo,
) *CatalogHandler {
	return &CatalogHandler{svc: svc, categoryR: cr, styleR: sr, brandR: br}
}

func (h *CatalogHandler) List(c *gin.Context) {
	var q domain.ListProductsQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		httpx.Error(c, http.StatusBadRequest, "INVALID_QUERY", err.Error())
		return
	}
	if q.Sort == "" {
		if q.Q != "" {
			q.Sort = "relevance"
		} else {
			q.Sort = "newest"
		}
	}
	items, total, suggestions, err := h.svc.List(c.Request.Context(), &q)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	out := make([]domain.ProductSummary, 0, len(items))
	for _, it := range items {
		out = append(out, domain.ToProductSummary(it))
	}
	resp := gin.H{
		"items":      out,
		"pagination": paginationEnvelope(q.Page, q.Limit, total),
	}
	if len(suggestions) > 0 {
		resp["suggestions"] = suggestions
	}
	httpx.OK(c, resp)
}

func (h *CatalogHandler) Detail(c *gin.Context) {
	bs := c.Param("brand_slug")
	ps := c.Param("product_slug")
	h.respondDetail(c, func() (*domain.Product, *domain.Category, []*domain.Variant, []*domain.Image, []*domain.StyleTag, error) {
		return h.svc.Detail(c.Request.Context(), bs, ps)
	})
}

func (h *CatalogHandler) DetailByID(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid product id")
		return
	}
	h.respondDetail(c, func() (*domain.Product, *domain.Category, []*domain.Variant, []*domain.Image, []*domain.StyleTag, error) {
		return h.svc.DetailByID(c.Request.Context(), id)
	})
}

func (h *CatalogHandler) respondDetail(c *gin.Context, fetch func() (*domain.Product, *domain.Category, []*domain.Variant, []*domain.Image, []*domain.StyleTag, error)) {
	p, cat, variants, images, tags, err := fetch()
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	vresp := make([]domain.VariantResp, 0, len(variants))
	for _, v := range variants {
		vresp = append(vresp, domain.ToVariantResp(v))
	}
	iresp := make([]domain.ImageResp, 0, len(images))
	for _, i := range images {
		iresp = append(iresp, domain.ToImageResp(i))
	}
	tresp := make([]domain.StyleTagRef, 0, len(tags))
	for _, t := range tags {
		tresp = append(tresp, domain.StyleTagRef{
			ID: t.ID.String(), Slug: t.Slug, Name: t.Name,
		})
	}
	brandRef := &domain.BrandRef{ID: p.BrandID.String()}
	if b, err := h.brandR.FindByID(c.Request.Context(), p.BrandID); err == nil {
		brandRef.Slug = b.Slug
		brandRef.Name = b.Name
	} else {
		log.Printf("catalog: brand lookup for %s failed: %v", p.BrandID, err)
	}
	out := domain.ProductDetail{
		ID: p.ID.String(), Slug: p.Slug, Name: p.Name,
		Description: p.Description, Status: string(p.Status),
		Currency: p.Currency,
		Brand:    brandRef,
		Category: &domain.CategoryRef{
			ID: cat.ID.String(), Slug: cat.Slug, Name: cat.Name,
		},
		StyleTags: tresp,
		Variants:  vresp,
		Images:    iresp,
		CreatedAt: domain.FormatTime(p.CreatedAt),
	}
	httpx.OK(c, gin.H{"product": out})
}

func (h *CatalogHandler) ListCategories(c *gin.Context) {
	items, err := h.categoryR.List(c.Request.Context())
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	out := make([]domain.CategoryRef, 0, len(items))
	for _, x := range items {
		out = append(out, domain.CategoryRef{
			ID: x.ID.String(), Slug: x.Slug, Name: x.Name,
		})
	}
	httpx.OK(c, gin.H{"items": out})
}

func (h *CatalogHandler) ListStyleTags(c *gin.Context) {
	items, err := h.styleR.List(c.Request.Context())
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	out := make([]domain.StyleTagRef, 0, len(items))
	for _, x := range items {
		out = append(out, domain.StyleTagRef{
			ID: x.ID.String(), Slug: x.Slug, Name: x.Name,
		})
	}
	httpx.OK(c, gin.H{"items": out})
}
