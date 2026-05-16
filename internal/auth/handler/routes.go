package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/wearwhere/wearwhere_be/internal/auth/middleware"
	jwtsvc "github.com/wearwhere/wearwhere_be/internal/shared/jwt"
)

type Deps struct {
	Auth     *AuthHandler
	Password *PasswordHandler
	OTP      *OTPHandler
	Social   *SocialHandler
	Profile  *ProfileHandler

	JWTIssuer *jwtsvc.Issuer
}

// Mount registers all auth routes onto the given router group.
// `auth` is the group whose middleware chain (e.g. rate limit) is already set.
func Mount(rg *gin.RouterGroup, d *Deps) {
	authGroup := rg.Group("/auth")
	{
		authGroup.POST("/register", d.Auth.Register)
		authGroup.POST("/login", d.Auth.Login)
		authGroup.POST("/refresh", d.Auth.Refresh)

		// Role-gated portals (UC41 brand, UC52 admin).
		authGroup.POST("/brand/login", d.Auth.BrandLogin)
		authGroup.POST("/admin/login", d.Auth.AdminLogin)

		authGroup.POST("/password/forgot", d.Password.Forgot)
		authGroup.POST("/password/reset", d.Password.Reset)

		authGroup.POST("/otp/send", d.OTP.Send)
		authGroup.POST("/otp/verify", d.OTP.Verify)

		authGroup.POST("/oauth/google", d.Social.Google)
		authGroup.POST("/oauth/apple", d.Social.Apple)

		// /logout requires a valid access token (so we know who is requesting),
		// and the body carries the refresh token to revoke.
		authGroup.POST("/logout",
			middleware.RequireAuth(d.JWTIssuer), d.Auth.Logout)
	}

	me := rg.Group("/me", middleware.RequireAuth(d.JWTIssuer))
	{
		me.GET("", d.Profile.Me)
		me.PATCH("", d.Profile.Update)
		me.DELETE("", d.Profile.Delete)
		me.POST("/password", d.Password.Change)
	}
}
