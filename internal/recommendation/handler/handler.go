package handler

import (
	"context"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	authmw "github.com/wearwhere/wearwhere_be/internal/auth/middleware"
	"github.com/wearwhere/wearwhere_be/internal/recommendation/domain"
	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

// Recommender is the service capability the handler needs (satisfied by
// recommendation/service.Service).
type Recommender interface {
	Recommend(ctx context.Context, userID uuid.UUID, limit int) (*domain.RecommendationsResponse, error)
}

type Handler struct{ svc Recommender }

func New(svc Recommender) *Handler { return &Handler{svc: svc} }

func (h *Handler) userID(c *gin.Context) uuid.UUID {
	id, _ := authmw.UserID(c)
	return id
}

func (h *Handler) List(c *gin.Context) {
	limit := 0
	if raw := c.Query("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = n
		}
	}
	resp, err := h.svc.Recommend(c.Request.Context(), h.userID(c), limit)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, resp)
}
