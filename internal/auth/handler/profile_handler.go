package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/wearwhere/wearwhere_be/internal/auth/domain"
	"github.com/wearwhere/wearwhere_be/internal/auth/middleware"
	"github.com/wearwhere/wearwhere_be/internal/auth/service"
	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

type ProfileHandler struct {
	svc *service.ProfileService
}

func NewProfileHandler(s *service.ProfileService) *ProfileHandler {
	return &ProfileHandler{svc: s}
}

func (h *ProfileHandler) Me(c *gin.Context) {
	uid, ok := middleware.UserID(c)
	if !ok {
		httpx.ErrorFromApp(c, domain.ErrUnauthorized)
		return
	}
	u, err := h.svc.Get(c, uid)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, gin.H{"user": domain.ToUserResponse(u)})
}

func (h *ProfileHandler) Update(c *gin.Context) {
	uid, ok := middleware.UserID(c)
	if !ok {
		httpx.ErrorFromApp(c, domain.ErrUnauthorized)
		return
	}
	var req domain.UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
		return
	}
	u, err := h.svc.Update(c, uid, req)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, gin.H{"user": domain.ToUserResponse(u)})
}

func (h *ProfileHandler) Delete(c *gin.Context) {
	uid, ok := middleware.UserID(c)
	if !ok {
		httpx.ErrorFromApp(c, domain.ErrUnauthorized)
		return
	}
	var req domain.DeleteAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
		return
	}
	// hasPendingOrders=nil for now; wire to the orders module when it lands.
	if err := h.svc.Delete(c, uid, req.Password, nil); err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, gin.H{"message": "Account scheduled for deletion within 90 days"})
}
