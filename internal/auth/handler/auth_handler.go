package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/wearwhere/wearwhere_be/internal/auth/domain"
	"github.com/wearwhere/wearwhere_be/internal/auth/service"
	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

type AuthHandler struct {
	auth *service.AuthService
}

func NewAuthHandler(a *service.AuthService) *AuthHandler { return &AuthHandler{auth: a} }

func (h *AuthHandler) Register(c *gin.Context) {
	var req domain.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
		return
	}
	resp, err := h.auth.Register(c, req, c.Request.UserAgent(), c.ClientIP())
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.Created(c, resp)
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req domain.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
		return
	}
	resp, err := h.auth.Login(c, req, c.Request.UserAgent(), c.ClientIP())
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, resp)
}

func (h *AuthHandler) BrandLogin(c *gin.Context) { h.loginAs(c, domain.RoleBrand) }
func (h *AuthHandler) AdminLogin(c *gin.Context) { h.loginAs(c, domain.RoleAdmin) }

func (h *AuthHandler) loginAs(c *gin.Context, role domain.Role) {
	var req domain.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
		return
	}
	resp, err := h.auth.LoginAs(c, role, req, c.Request.UserAgent(), c.ClientIP())
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, resp)
}

func (h *AuthHandler) Refresh(c *gin.Context) {
	var req domain.RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
		return
	}
	tokens, err := h.auth.Refresh(c, req.RefreshToken, c.Request.UserAgent(), c.ClientIP())
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, gin.H{"tokens": tokens})
}

func (h *AuthHandler) Logout(c *gin.Context) {
	var req domain.LogoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
		return
	}
	if err := h.auth.Logout(c, req.RefreshToken); err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.NoContent(c)
}
