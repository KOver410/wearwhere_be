package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/wearwhere/wearwhere_be/internal/auth/domain"
	"github.com/wearwhere/wearwhere_be/internal/auth/service"
	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

type SocialHandler struct {
	svc *service.SocialService
}

func NewSocialHandler(s *service.SocialService) *SocialHandler { return &SocialHandler{svc: s} }

func (h *SocialHandler) loginWith(c *gin.Context, provider domain.OAuthProvider) {
	var req domain.SocialLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
		return
	}
	resp, err := h.svc.LoginOrRegister(c, provider, req.IDToken, c.Request.UserAgent(), c.ClientIP())
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, resp)
}

func (h *SocialHandler) Google(c *gin.Context) { h.loginWith(c, domain.ProviderGoogle) }
func (h *SocialHandler) Apple(c *gin.Context)  { h.loginWith(c, domain.ProviderApple) }
