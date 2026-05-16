package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/wearwhere/wearwhere_be/internal/auth/domain"
	"github.com/wearwhere/wearwhere_be/internal/auth/service"
	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

type OTPHandler struct {
	otp  *service.OTPService
	auth *service.AuthService
}

func NewOTPHandler(o *service.OTPService, a *service.AuthService) *OTPHandler {
	return &OTPHandler{otp: o, auth: a}
}

func (h *OTPHandler) Send(c *gin.Context) {
	var req domain.SendOTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
		return
	}
	channel, contact := "email", req.Email
	if req.Phone != "" {
		channel, contact = "sms", req.Phone
	}
	if contact == "" {
		httpx.ErrorFromApp(c, domain.ErrEmailOrPhoneRequired)
		return
	}
	if err := h.otp.Send(c, channel, contact, req.Purpose); err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, gin.H{"message": "OTP sent"})
}

func (h *OTPHandler) Verify(c *gin.Context) {
	var req domain.VerifyOTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
		return
	}
	contact := req.Email
	if contact == "" {
		contact = req.Phone
	}
	if contact == "" {
		httpx.ErrorFromApp(c, domain.ErrEmailOrPhoneRequired)
		return
	}
	if err := h.otp.Verify(c, contact, req.Purpose, req.OTP); err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}

	// Side effect: flip the verified flag on the user record
	if req.Purpose == service.PurposeVerifyEmail || req.Purpose == service.PurposeVerifyPhone {
		if err := h.markVerified(c, req); err != nil && !errors.Is(err, domain.ErrUserNotFound) {
			httpx.ErrorFromApp(c, err)
			return
		}
	}
	httpx.OK(c, gin.H{"message": "OTP verified"})
}

func (h *OTPHandler) markVerified(ctx context.Context, req domain.VerifyOTPRequest) error {
	user, err := h.auth.FindUserForVerification(ctx, req.Email, req.Phone)
	if err != nil {
		return err
	}
	if req.Purpose == service.PurposeVerifyEmail {
		return h.markEmail(ctx, user)
	}
	return h.markPhone(ctx, user)
}

// markEmail / markPhone delegate to the user repo through AuthService — we
// keep handler-level dependencies thin by leaning on the public service API.
func (h *OTPHandler) markEmail(ctx context.Context, u *domain.User) error {
	return h.auth.MarkEmailVerified(ctx, u.ID)
}
func (h *OTPHandler) markPhone(ctx context.Context, u *domain.User) error {
	return h.auth.MarkPhoneVerified(ctx, u.ID)
}
