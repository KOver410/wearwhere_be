package handler

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	authmw "github.com/wearwhere/wearwhere_be/internal/auth/middleware"
	"github.com/wearwhere/wearwhere_be/internal/wardrobe/domain"
	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

// WardrobeService is the capability the handler needs.
type WardrobeService interface {
	Get(ctx context.Context, userID uuid.UUID) (*domain.WardrobeResponse, error)
	Regenerate(ctx context.Context, userID uuid.UUID) (*domain.WardrobeResponse, error)
}

type Handler struct{ svc WardrobeService }

func New(svc WardrobeService) *Handler { return &Handler{svc: svc} }

func (h *Handler) userID(c *gin.Context) uuid.UUID {
	id, _ := authmw.UserID(c)
	return id
}

func (h *Handler) Get(c *gin.Context) {
	resp, err := h.svc.Get(c.Request.Context(), h.userID(c))
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, resp)
}

func (h *Handler) Regenerate(c *gin.Context) {
	resp, err := h.svc.Regenerate(c.Request.Context(), h.userID(c))
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, resp)
}
