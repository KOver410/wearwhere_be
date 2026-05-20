package handler

import "github.com/gin-gonic/gin"

// MountBrandProducts mounts /brand/me/products under the given group.
// Caller is responsible for applying RequireAuth + RequireRole + BrandContext.
func MountBrandProducts(rg *gin.RouterGroup, h *BrandProductHandler) {
	p := rg.Group("/products")
	{
		p.GET("", h.List)
		p.POST("", h.Create)
		p.GET(":id", h.Get)
		p.PATCH(":id", h.Update)
		p.DELETE(":id", h.Delete)

		p.POST(":id/variants", h.CreateVariant)
		p.PATCH(":id/variants/:variant_id", h.UpdateVariant)
		p.DELETE(":id/variants/:variant_id", h.DeleteVariant)

		p.POST(":id/images", h.UploadImages)
		p.PATCH(":id/images/:image_id", h.UpdateImage)
		p.DELETE(":id/images/:image_id", h.DeleteImage)
	}
}

// MountCatalog mounts public read-only catalog routes (no auth required).
func MountCatalog(rg *gin.RouterGroup, h *CatalogHandler) {
	rg.GET("/products", h.List)
	rg.GET("/products/by-id/:id", h.DetailByID)
	rg.GET("/brands/:brand_slug/products/:product_slug", h.Detail)
	rg.GET("/categories", h.ListCategories)
	rg.GET("/style-tags", h.ListStyleTags)
}
