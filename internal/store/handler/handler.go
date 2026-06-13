// Package handler exposes public store-discovery HTTP endpoints.
package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/wearwhere/wearwhere_be/internal/maps/goong"
	"github.com/wearwhere/wearwhere_be/internal/store/repo"
	"github.com/wearwhere/wearwhere_be/internal/store/service"
	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

type Handler struct{ svc *service.Service }

func NewHandler(svc *service.Service) *Handler { return &Handler{svc: svc} }

// parseLatLng parses lat and lng strings; returns ok=false on failure.
func parseLatLng(lat, lng string) (goong.LatLng, bool) {
	la, err1 := strconv.ParseFloat(strings.TrimSpace(lat), 64)
	ln, err2 := strconv.ParseFloat(strings.TrimSpace(lng), 64)
	if err1 != nil || err2 != nil {
		return goong.LatLng{}, false
	}
	return goong.LatLng{Lat: la, Lng: ln}, true
}

func (h *Handler) Nearby(c *gin.Context) {
	p, ok := parseLatLng(c.Query("lat"), c.Query("lng"))
	if !ok {
		httpx.Error(c, http.StatusBadRequest, "INVALID_QUERY", "lat and lng are required floats")
		return
	}
	radius, _ := strconv.ParseFloat(c.Query("radius_km"), 64)
	items, err := h.svc.Nearby(c.Request.Context(), p.Lat, p.Lng, radius)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

func (h *Handler) Search(c *gin.Context) {
	f := repo.AreaFilter{
		CityCode:     c.Query("city_code"),
		DistrictCode: c.Query("district_code"),
		WardCode:     c.Query("ward_code"),
		Q:            c.Query("q"),
		Limit:        50,
	}
	var origin *goong.LatLng
	if p, ok := parseLatLng(c.Query("lat"), c.Query("lng")); ok {
		origin = &p
	}
	items, err := h.svc.SearchByArea(c.Request.Context(), f, origin)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

func (h *Handler) Detail(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, "INVALID_ID", "invalid store id")
		return
	}
	d, err := h.svc.Detail(c.Request.Context(), id)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, d)
}

func (h *Handler) Directions(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, "INVALID_ID", "invalid store id")
		return
	}
	from := c.Query("from")
	parts := strings.Split(from, ",")
	if len(parts) != 2 {
		httpx.Error(c, http.StatusBadRequest, "INVALID_QUERY", "from must be 'lat,lng'")
		return
	}
	p, ok := parseLatLng(parts[0], parts[1])
	if !ok {
		httpx.Error(c, http.StatusBadRequest, "INVALID_QUERY", "from must be 'lat,lng'")
		return
	}
	resp, err := h.svc.Directions(c.Request.Context(), id, p)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, resp)
}
