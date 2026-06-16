package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	authmw "github.com/wearwhere/wearwhere_be/internal/auth/middleware"
	"github.com/wearwhere/wearwhere_be/internal/styleprofile/domain"
	"github.com/wearwhere/wearwhere_be/internal/styleprofile/service"
	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

type Handler struct{ svc *service.Service }

func New(s *service.Service) *Handler { return &Handler{svc: s} }

func (h *Handler) userID(c *gin.Context) uuid.UUID {
	id, _ := authmw.UserID(c)
	return id
}

func viewToResponse(v *domain.StyleProfileView) domain.StyleProfileResponse {
	resp := domain.StyleProfileResponse{
		StyleTags: v.StyleTags,
		BudgetMin: v.BudgetMin,
		BudgetMax: v.BudgetMax,
	}
	if resp.StyleTags == nil {
		resp.StyleTags = []domain.StyleTagRef{}
	}
	if v.OnboardedAt != nil {
		s := v.OnboardedAt.UTC().Format("2006-01-02T15:04:05Z")
		resp.OnboardedAt = &s
	}
	return resp
}

func (h *Handler) Get(c *gin.Context) {
	v, err := h.svc.Get(c.Request.Context(), h.userID(c))
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, viewToResponse(v))
}

func (h *Handler) Put(c *gin.Context) {
	var req domain.UpdateStyleProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
		return
	}
	v, err := h.svc.Save(c.Request.Context(), h.userID(c), req)
	if err != nil {
		var ute *domain.UnknownStyleTagsError
		if errors.As(err, &ute) {
			httpx.ErrorWithDetails(c, http.StatusBadRequest, "VALIDATION_FAILED",
				"One or more style tags do not exist",
				map[string]any{"unknown_style_tag_ids": ute.IDs})
			return
		}
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, viewToResponse(v))
}
