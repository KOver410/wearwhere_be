package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/wearwhere/wearwhere_be/internal/auth/handler"
	"github.com/wearwhere/wearwhere_be/internal/auth/middleware"
	"github.com/wearwhere/wearwhere_be/internal/auth/repo"
	"github.com/wearwhere/wearwhere_be/internal/auth/service"
	"github.com/wearwhere/wearwhere_be/internal/config"
	jwtsvc "github.com/wearwhere/wearwhere_be/internal/shared/jwt"
	"github.com/wearwhere/wearwhere_be/internal/shared/mailer"
	"github.com/wearwhere/wearwhere_be/internal/shared/postgres"
	rediscli "github.com/wearwhere/wearwhere_be/internal/shared/redis"
	"github.com/wearwhere/wearwhere_be/internal/shared/sms"
	authvalidator "github.com/wearwhere/wearwhere_be/internal/shared/validator"

	authdomain "github.com/wearwhere/wearwhere_be/internal/auth/domain"
	brandhandler "github.com/wearwhere/wearwhere_be/internal/brand/handler"
	brandmw "github.com/wearwhere/wearwhere_be/internal/brand/middleware"
	brandrepo "github.com/wearwhere/wearwhere_be/internal/brand/repo"
	brandservice "github.com/wearwhere/wearwhere_be/internal/brand/service"
	producthandler "github.com/wearwhere/wearwhere_be/internal/product/handler"
	productrepo "github.com/wearwhere/wearwhere_be/internal/product/repo"
	productservice "github.com/wearwhere/wearwhere_be/internal/product/service"
	"github.com/wearwhere/wearwhere_be/internal/shared/storage"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	if cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}
	authvalidator.RegisterWithGin()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	pgPool, err := postgres.New(ctx, cfg.DB)
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}
	defer pgPool.Close()

	rdb, err := rediscli.New(ctx, cfg.Redis)
	if err != nil {
		log.Fatalf("redis: %v", err)
	}
	defer rdb.Close()

	// ── shared singletons ──
	jwtIssuer := jwtsvc.NewIssuer(cfg.JWT.Secret, cfg.JWT.AccessTTL)
	mailerSvc := mailer.NewSMTP(cfg.SMTP)
	smsSvc := sms.NewTwilio(cfg.SMS)

	// ── repos ──
	userRepo := repo.NewUserPG(pgPool)
	sessionRepo := repo.NewSessionPG(pgPool)
	otpStore := repo.NewOTPRedis(rdb)
	attemptStore := repo.NewAttemptRedis(rdb)
	brandRepo := brandrepo.NewBrandPG(pgPool)
	addressRepo := brandrepo.NewAddressPG(pgPool)
	productRepo := productrepo.NewProductPG(pgPool)
	variantRepo := productrepo.NewVariantPG(pgPool)
	imageRepo := productrepo.NewImagePG(pgPool)
	categoryRepo := productrepo.NewCategoryPG(pgPool)
	styleTagRepo := productrepo.NewStyleTagPG(pgPool)

	// ── storage ──
	storageBackend, err := storage.New(storage.Config{
		Driver:         cfg.Storage.Driver,
		LocalDir:       cfg.Storage.LocalDir,
		BaseURL:        cfg.Storage.BaseURL,
		GCSBucket:      cfg.Storage.GCSBucket,
		GCSCredentials: cfg.Storage.GCSCredentials,
		MaxFileSize:    cfg.Storage.MaxFileSize,
		AllowedMIMEs:   cfg.Storage.AllowedMIMEs,
	})
	if err != nil {
		log.Fatalf("storage: %v", err)
	}

	// ── services ──
	tokenSvc := service.NewTokenService(jwtIssuer, sessionRepo, cfg.JWT.RefreshTTL)
	otpSvc := service.NewOTPService(otpStore, mailerSvc, smsSvc, cfg.Limit)
	authSvc := service.NewAuthService(userRepo, attemptStore, tokenSvc, otpSvc, cfg.Limit)
	passwordSvc := service.NewPasswordService(userRepo, sessionRepo, otpSvc, authSvc)
	profileSvc := service.NewProfileService(userRepo, sessionRepo)
	socialSvc := service.NewSocialService(userRepo, tokenSvc, cfg.OAuth)
	brandSvc := brandservice.New(brandRepo, addressRepo)
	productSvc := productservice.New(
		productRepo, variantRepo, imageRepo,
		categoryRepo, styleTagRepo,
		storageBackend, cfg.Storage.AllowedMIMEs, cfg.Storage.MaxFileSize,
	)

	// ── handlers ──
	deps := &handler.Deps{
		Auth:      handler.NewAuthHandler(authSvc),
		Password:  handler.NewPasswordHandler(passwordSvc),
		OTP:       handler.NewOTPHandler(otpSvc, authSvc),
		Social:    handler.NewSocialHandler(socialSvc),
		Profile:   handler.NewProfileHandler(profileSvc),
		JWTIssuer: jwtIssuer,
	}
	brandDeps := &brandhandler.Deps{
		Brand:   brandhandler.NewBrandHandler(brandSvc),
		Address: brandhandler.NewAddressHandler(brandSvc),
	}

	brandProductHandler := producthandler.NewBrandProductHandler(productSvc)

	// ── router ──
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(gin.Logger())

	r.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })

	if cfg.Storage.Driver == "local" || cfg.Storage.Driver == "" {
		r.Static("/uploads", cfg.Storage.LocalDir)
	}

	v1 := r.Group("/api/v1",
		middleware.OptionalAuth(jwtIssuer),
		middleware.RateLimit(rdb, cfg.Limit.RateLimitPerMin),
	)
	handler.Mount(v1, deps)

	brandGroup := v1.Group("/brand/me",
		middleware.RequireAuth(jwtIssuer),
		middleware.RequireRole(authdomain.RoleBrand),
		brandmw.BrandContext(brandRepo),
	)
	brandhandler.Mount(brandGroup, brandDeps)
	producthandler.MountBrandProducts(brandGroup, brandProductHandler)

	srv := &http.Server{
		Addr:         ":" + cfg.HTTP.Port,
		Handler:      r,
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
	}

	go func() {
		log.Printf("listening on :%s (env=%s)", cfg.HTTP.Port, cfg.App.Env)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutdown signal received")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
	_ = os.Stdout.Sync()
}
