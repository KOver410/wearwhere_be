// Package middleware: BrandContext loads the caller's brand and attaches
// brand_id to the request context. Chain after RequireAuth + RequireRole.
package middleware

import (
	"errors"

	"github.com/gin-gonic/gin"

	authmw "github.com/wearwhere/wearwhere_be/internal/auth/middleware"
	"github.com/wearwhere/wearwhere_be/internal/brand/domain"
	"github.com/wearwhere/wearwhere_be/internal/brand/repo"
	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

const (
	CtxBrandID = "brand.id"
	CtxBrand   = "brand.entity"
)

func BrandContext(brandRepo repo.BrandRepo) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid, ok := authmw.UserID(c)
		if !ok {
			httpx.Error(c, 401, "UNAUTHORIZED", "Authentication required")
			return
		}
		b, err := brandRepo.FindByOwnerUserID(c.Request.Context(), uid)
		switch {
		case errors.Is(err, repo.ErrNotFound):
			httpx.ErrorFromApp(c, domain.ErrNoBrandOwned)
			return
		case err != nil:
			httpx.ErrorFromApp(c, domain.ErrBrandNotFound)
			return
		case b.Status == domain.BrandStatusSuspended:
			httpx.ErrorFromApp(c, domain.ErrBrandSuspended)
			return
		}
		c.Set(CtxBrandID, b.ID)
		c.Set(CtxBrand, b)
		c.Next()
	}
}
