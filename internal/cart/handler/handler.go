// Package handler exposes HTTP endpoints for the shopping cart.
package handler

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	authmw "github.com/wearwhere/wearwhere_be/internal/auth/middleware"
	"github.com/wearwhere/wearwhere_be/internal/cart/domain"
	"github.com/wearwhere/wearwhere_be/internal/cart/service"
	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

type Handler struct{ svc *service.Service }

func New(s *service.Service) *Handler { return &Handler{svc: s} }

func (h *Handler) userID(c *gin.Context) uuid.UUID {
	id, _ := authmw.UserID(c)
	return id
}

func money(v float64) string { return fmt.Sprintf("%.2f", v) }

func toItemResponse(v *domain.CartItemView) (domain.CartItemResponse, float64, float64) {
	subSnap := v.PriceSnapshot * float64(v.Qty)
	subCur := v.CurrentPrice * float64(v.Qty)
	out := domain.CartItemResponse{
		ID:                v.ID.String(),
		Qty:               v.Qty,
		PriceSnapshot:     money(v.PriceSnapshot),
		CurrentPrice:      money(v.CurrentPrice),
		PriceChanged:      v.PriceSnapshot != v.CurrentPrice,
		SubtotalSnapshot:  money(subSnap),
		SubtotalCurrent:   money(subCur),
		Currency:          v.CurrencySnapshot,
		Unavailable:       v.Unavailable,
		UnavailableReason: v.UnavailableReason,
		AddedAt:           v.AddedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
	out.Variant.ID = v.VariantID.String()
	out.Variant.SKU = v.SKU
	out.Variant.Size = v.Size
	out.Variant.Color = v.Color
	out.Variant.ColorHex = v.ColorHex
	out.Variant.StockQty = v.StockQty
	out.Product.ID = v.ProductID.String()
	out.Product.Slug = v.ProductSlug
	out.Product.Name = v.ProductName
	out.Product.PrimaryImageURL = v.PrimaryImageURL
	out.Brand.ID = v.BrandID.String()
	out.Brand.Slug = v.BrandSlug
	out.Brand.Name = v.BrandName
	return out, subSnap, subCur
}

func (h *Handler) Get(c *gin.Context) {
	items, err := h.svc.List(c.Request.Context(), h.userID(c))
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	resp := domain.CartResponse{Items: make([]domain.CartItemResponse, 0, len(items))}
	currency := "VND"
	var totalSnap, totalCur float64
	summary := domain.CartSummary{}
	for _, v := range items {
		if v.CurrencySnapshot != "" {
			currency = v.CurrencySnapshot
		}
		item, subSnap, subCur := toItemResponse(v)
		resp.Items = append(resp.Items, item)
		totalSnap += subSnap
		totalCur += subCur
		summary.TotalQty += item.Qty
		if item.PriceChanged {
			summary.HasPriceChanges = true
		}
		if item.Unavailable {
			summary.HasUnavailable = true
		}
	}
	summary.ItemCount = len(resp.Items)
	summary.TotalSnapshot = money(totalSnap)
	summary.TotalCurrent = money(totalCur)
	summary.Currency = currency
	resp.Summary = summary
	httpx.OK(c, resp)
}

func (h *Handler) Add(c *gin.Context) {
	var req domain.AddToCartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	vid, err := uuid.Parse(req.VariantID)
	if err != nil {
		httpx.ErrorFromApp(c, domain.ErrVariantUnavailable)
		return
	}
	item, err := h.svc.Add(c.Request.Context(), h.userID(c), vid, req.Qty)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.Created(c, gin.H{"id": item.ID.String(), "qty": item.Qty})
}

func (h *Handler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("item_id"))
	if err != nil {
		httpx.ErrorFromApp(c, domain.ErrCartItemNotFound)
		return
	}
	var req domain.UpdateCartItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	item, err := h.svc.UpdateQty(c.Request.Context(), id, h.userID(c), req.Qty)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, gin.H{"id": item.ID.String(), "qty": item.Qty})
}

func (h *Handler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("item_id"))
	if err != nil {
		httpx.ErrorFromApp(c, domain.ErrCartItemNotFound)
		return
	}
	if err := h.svc.Remove(c.Request.Context(), id, h.userID(c)); err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.NoContent(c)
}

func (h *Handler) Clear(c *gin.Context) {
	if err := h.svc.Clear(c.Request.Context(), h.userID(c)); err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.NoContent(c)
}
