package handler

import (
	"github.com/gin-gonic/gin"
)

type Deps struct {
	Brand   *BrandHandler
	Address *AddressHandler
}

// Mount registers /brand/me routes on the given group.
// Caller is responsible for chaining RequireAuth + RequireRole + BrandContext
// onto the group before calling Mount.
func Mount(rg *gin.RouterGroup, d *Deps) {
	rg.GET("", d.Brand.Me)
	rg.PATCH("", d.Brand.UpdateMe)

	addr := rg.Group("/addresses")
	{
		addr.GET("", d.Address.List)
		addr.POST("", d.Address.Create)
		addr.PATCH(":id", d.Address.Update)
		addr.DELETE(":id", d.Address.Delete)
	}
}

// MountBrandsPublic registers public read-only brand routes (no auth required).
func MountBrandsPublic(rg *gin.RouterGroup, h *BrandsPublicHandler) {
	rg.GET("/brands", h.List)
	rg.GET("/brands/:slug", h.Detail)
}
