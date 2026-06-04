package location

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts location endpoints under the given authenticated group.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler) {
	loc := rg.Group("/locations")
	loc.GET("/cities", h.Cities)
	loc.GET("/cities/:city_code/districts", h.Districts)
	loc.GET("/districts/:district_code/wards", h.Wards)
}
