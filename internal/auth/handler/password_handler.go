package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/wearwhere/wearwhere_be/internal/auth/domain"
	"github.com/wearwhere/wearwhere_be/internal/auth/middleware"
	"github.com/wearwhere/wearwhere_be/internal/auth/service"
	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

type PasswordHandler struct {
	svc *service.PasswordService
}

func NewPasswordHandler(s *service.PasswordService) *PasswordHandler {
	return &PasswordHandler{svc: s}
}

func (h *PasswordHandler) Forgot(c *gin.Context) {
	var req domain.ForgotPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
		return
	}
	if err := h.svc.Forgot(c, req); err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, gin.H{"message": "If the account exists, an OTP has been sent"})
}

func (h *PasswordHandler) Reset(c *gin.Context) {
	var req domain.ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
		return
	}
	if err := h.svc.Reset(c, req); err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, gin.H{"message": "Password reset successfully"})
}

func (h *PasswordHandler) Change(c *gin.Context) {
	uid, ok := middleware.UserID(c)
	if !ok {
		httpx.ErrorFromApp(c, domain.ErrUnauthorized)
		return
	}
	var req domain.ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
		return
	}
	// keepRefreshHash="" → revoke ALL sessions. The client should re-login.
	if err := h.svc.Change(c, uid, req, ""); err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, gin.H{"message": "Password changed. Please log in again on other devices."})
}
