package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	authmw "github.com/wearwhere/wearwhere_be/internal/auth/middleware"
	"github.com/wearwhere/wearwhere_be/internal/wishlist/domain"
	"github.com/wearwhere/wearwhere_be/internal/wishlist/service"
	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

type Handler struct{ svc *service.Service }

func New(s *service.Service) *Handler { return &Handler{svc: s} }

func (h *Handler) userID(c *gin.Context) uuid.UUID {
	id, _ := authmw.UserID(c)
	return id
}

func itemToResponse(v *domain.WishlistItemView) domain.WishlistItemResponse {
	r := domain.WishlistItemResponse{
		ProductID:       v.ProductID.String(),
		ProductSlug:     v.ProductSlug,
		ProductName:     v.ProductName,
		PrimaryImageURL: v.PrimaryImageURL,
		MinPrice:        v.MinPrice,
		AddedAt:         v.AddedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
	r.Brand.ID = v.BrandID.String()
	r.Brand.Slug = v.BrandSlug
	r.Brand.Name = v.BrandName
	return r
}

func (h *Handler) Add(c *gin.Context) {
	pid, err := uuid.Parse(c.Param("product_id"))
	if err != nil {
		httpx.ErrorFromApp(c, domain.ErrProductNotFound)
		return
	}
	if err := h.svc.Add(c.Request.Context(), h.userID(c), pid); err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, gin.H{"in_wishlist": true})
}

func (h *Handler) Remove(c *gin.Context) {
	pid, err := uuid.Parse(c.Param("product_id"))
	if err != nil {
		// Idempotent: bad UUID still 204.
		httpx.NoContent(c)
		return
	}
	if err := h.svc.Remove(c.Request.Context(), h.userID(c), pid); err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.NoContent(c)
}

func (h *Handler) List(c *gin.Context) {
	var q domain.WishlistListQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		httpx.Error(c, http.StatusBadRequest, "INVALID_QUERY", err.Error())
		return
	}
	items, total, err := h.svc.List(c.Request.Context(), h.userID(c), q.Page, q.Limit)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	resp := domain.WishlistListResponse{Items: make([]domain.WishlistItemResponse, 0, len(items))}
	for _, v := range items {
		resp.Items = append(resp.Items, itemToResponse(v))
	}
	resp.Pagination.Page = q.Page
	resp.Pagination.Limit = q.Limit
	resp.Pagination.Total = total
	if q.Limit > 0 {
		resp.Pagination.TotalPages = (total + q.Limit - 1) / q.Limit
	}
	resp.Pagination.HasMore = q.Page*q.Limit < total
	httpx.OK(c, resp)
}

func (h *Handler) Contains(c *gin.Context) {
	var q domain.WishlistContainsQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		httpx.Error(c, http.StatusBadRequest, "INVALID_QUERY", err.Error())
		return
	}
	ids := make([]uuid.UUID, 0, len(q.ProductIDs))
	for _, s := range q.ProductIDs {
		if id, err := uuid.Parse(s); err == nil {
			ids = append(ids, id)
		}
	}
	res, err := h.svc.Contains(c.Request.Context(), h.userID(c), ids)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	out := make(map[string]bool, len(res))
	for k, v := range res {
		out[k.String()] = v
	}
	httpx.OK(c, domain.WishlistContainsResponse{InWishlist: out})
}
