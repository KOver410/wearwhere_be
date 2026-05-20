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
