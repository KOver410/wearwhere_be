package location

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

type Handler struct{ svc *Service }

func NewHandler(s *Service) *Handler { return &Handler{svc: s} }

func (h *Handler) Cities(c *gin.Context) {
	out, err := h.svc.Cities(c.Request.Context())
	if err != nil {
		httpx.Error(c, http.StatusBadGateway, "GOSHIP_UNAVAILABLE", "failed to load cities")
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

func (h *Handler) Districts(c *gin.Context) {
	code := c.Param("city_code")
	if code == "" {
		httpx.Error(c, http.StatusBadRequest, "MISSING_PARAM", "missing city_code")
		return
	}
	out, err := h.svc.Districts(c.Request.Context(), code)
	if err != nil {
		httpx.Error(c, http.StatusBadGateway, "GOSHIP_UNAVAILABLE", "failed to load districts")
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

func (h *Handler) Wards(c *gin.Context) {
	code := c.Param("district_code")
	if code == "" {
		httpx.Error(c, http.StatusBadRequest, "MISSING_PARAM", "missing district_code")
		return
	}
	out, err := h.svc.Wards(c.Request.Context(), code)
	if err != nil {
		httpx.Error(c, http.StatusBadGateway, "GOSHIP_UNAVAILABLE", "failed to load wards")
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}
