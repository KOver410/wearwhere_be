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
	"github.com/google/uuid"

	"github.com/wearwhere/wearwhere_be/internal/auth/handler"
	"github.com/wearwhere/wearwhere_be/internal/auth/middleware"
	"github.com/wearwhere/wearwhere_be/internal/auth/repo"
	"github.com/wearwhere/wearwhere_be/internal/auth/service"
	"github.com/wearwhere/wearwhere_be/internal/config"
	"github.com/wearwhere/wearwhere_be/internal/shared/httpmw"
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
	carthandler "github.com/wearwhere/wearwhere_be/internal/cart/handler"
	cartrepo "github.com/wearwhere/wearwhere_be/internal/cart/repo"
	cartservice "github.com/wearwhere/wearwhere_be/internal/cart/service"
	customeraddrhandler "github.com/wearwhere/wearwhere_be/internal/customeraddr/handler"
	customeraddrrepo "github.com/wearwhere/wearwhere_be/internal/customeraddr/repo"
	customeraddrservice "github.com/wearwhere/wearwhere_be/internal/customeraddr/service"
	jobsmod "github.com/wearwhere/wearwhere_be/internal/jobs"
	orderhandler "github.com/wearwhere/wearwhere_be/internal/order/handler"
	orderrepo "github.com/wearwhere/wearwhere_be/internal/order/repo"
	orderservice "github.com/wearwhere/wearwhere_be/internal/order/service"
	paymenthandler "github.com/wearwhere/wearwhere_be/internal/payment/handler"
	"github.com/wearwhere/wearwhere_be/internal/payment/payos"
	paymentrepo "github.com/wearwhere/wearwhere_be/internal/payment/repo"
	paymentservice "github.com/wearwhere/wearwhere_be/internal/payment/service"
	producthandler "github.com/wearwhere/wearwhere_be/internal/product/handler"
	productrepo "github.com/wearwhere/wearwhere_be/internal/product/repo"
	productservice "github.com/wearwhere/wearwhere_be/internal/product/service"
	promohandler "github.com/wearwhere/wearwhere_be/internal/promo/handler"
	promorepo "github.com/wearwhere/wearwhere_be/internal/promo/repo"
	promoservice "github.com/wearwhere/wearwhere_be/internal/promo/service"
	"github.com/wearwhere/wearwhere_be/internal/shared/storage"
	"github.com/wearwhere/wearwhere_be/internal/shipping/goship"
	"github.com/wearwhere/wearwhere_be/internal/shipping/location"
	"github.com/wearwhere/wearwhere_be/internal/shipping/provider"
	"github.com/wearwhere/wearwhere_be/internal/shipping/weight"
	blockhandler "github.com/wearwhere/wearwhere_be/internal/block/handler"
	blockrepo "github.com/wearwhere/wearwhere_be/internal/block/repo"
	blockservice "github.com/wearwhere/wearwhere_be/internal/block/service"
	followhandler "github.com/wearwhere/wearwhere_be/internal/follow/handler"
	followrepo "github.com/wearwhere/wearwhere_be/internal/follow/repo"
	followservice "github.com/wearwhere/wearwhere_be/internal/follow/service"
	ootdhandler "github.com/wearwhere/wearwhere_be/internal/ootd/handler"
	ootdrepo "github.com/wearwhere/wearwhere_be/internal/ootd/repo"
	ootdservice "github.com/wearwhere/wearwhere_be/internal/ootd/service"
	reviewhandler "github.com/wearwhere/wearwhere_be/internal/review/handler"
	reviewrepo "github.com/wearwhere/wearwhere_be/internal/review/repo"
	reviewservice "github.com/wearwhere/wearwhere_be/internal/review/service"
	wishlisthandler "github.com/wearwhere/wearwhere_be/internal/wishlist/handler"
	wishlistrepo "github.com/wearwhere/wearwhere_be/internal/wishlist/repo"
	wishlistservice "github.com/wearwhere/wearwhere_be/internal/wishlist/service"
	styleprofilehandler "github.com/wearwhere/wearwhere_be/internal/styleprofile/handler"
	styleprofilerepo "github.com/wearwhere/wearwhere_be/internal/styleprofile/repo"
	styleprofileservice "github.com/wearwhere/wearwhere_be/internal/styleprofile/service"

	recommendationhandler "github.com/wearwhere/wearwhere_be/internal/recommendation/handler"
	recommendationrepo "github.com/wearwhere/wearwhere_be/internal/recommendation/repo"
	recommendationservice "github.com/wearwhere/wearwhere_be/internal/recommendation/service"

	"github.com/wearwhere/wearwhere_be/internal/shared/llm"
	wardrobehandler "github.com/wearwhere/wearwhere_be/internal/wardrobe/handler"
	wardroberepo "github.com/wearwhere/wearwhere_be/internal/wardrobe/repo"
	wardrobeservice "github.com/wearwhere/wearwhere_be/internal/wardrobe/service"

	"github.com/wearwhere/wearwhere_be/internal/maps/goong"
	storehandler "github.com/wearwhere/wearwhere_be/internal/store/handler"
	storerepo "github.com/wearwhere/wearwhere_be/internal/store/repo"
	storeservice "github.com/wearwhere/wearwhere_be/internal/store/service"
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
	customerAddrRepo := customeraddrrepo.NewAddressPG(pgPool)
	wishlistRepo := wishlistrepo.NewWishlistPG(pgPool)
	cartRepo := cartrepo.NewCartPG(pgPool)
	reviewRepo := reviewrepo.NewReviewPG(pgPool)
	reviewSvc := reviewservice.NewWithRepo(reviewRepo)
	reviewHandler := reviewhandler.New(reviewSvc)
	followSvc := followservice.New(followrepo.NewFollowPG(pgPool))
	followHandler := followhandler.New(followSvc)
	blockHandler := blockhandler.New(blockservice.New(blockrepo.NewBlockPG(pgPool)))

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

	// ── shipping client + location ──
	goshipClient, err := goship.NewFromConfig(goship.Config{
		Mode:         cfg.Goship.Mode,
		Token:        cfg.Goship.Token,
		ClientSecret: cfg.Goship.ClientSecret,
		BaseURL:      cfg.Goship.BaseURL,
	})
	if err != nil {
		log.Fatalf("goship client: %v", err)
	}
	locSvc := location.NewService(goshipClient, 24*time.Hour)

	// ── maps (Goong) ──
	goongClient, err := goong.NewFromConfig(goong.Config{
		Mode:    cfg.Goong.Mode,
		APIKey:  cfg.Goong.APIKey,
		BaseURL: cfg.Goong.BaseURL,
	})
	if err != nil {
		log.Fatalf("goong client: %v", err)
	}

	// ── LLM client (wardrobe) ──
	llmClient, err := llm.NewFromConfig(llm.Config{
		Provider: cfg.AI.Provider,
		APIKey:   cfg.AI.APIKey,
		Model:    cfg.AI.Model,
		BaseURL:  cfg.AI.BaseURL,
		Timeout:  cfg.AI.Timeout,
	})
	if err != nil {
		log.Fatalf("llm: %v", err)
	}

	// ── services ──
	tokenSvc := service.NewTokenService(jwtIssuer, sessionRepo, cfg.JWT.RefreshTTL)
	otpSvc := service.NewOTPService(otpStore, mailerSvc, smsSvc, cfg.Limit)
	authSvc := service.NewAuthService(userRepo, attemptStore, tokenSvc, otpSvc, cfg.Limit)
	passwordSvc := service.NewPasswordService(userRepo, sessionRepo, otpSvc, authSvc)
	profileSvc := service.NewProfileService(userRepo, sessionRepo)
	socialSvc := service.NewSocialService(userRepo, tokenSvc, cfg.OAuth)
	brandSvc := brandservice.New(brandRepo, addressRepo, locSvc)
	productSvc := productservice.New(
		productRepo, variantRepo, imageRepo,
		categoryRepo, styleTagRepo,
		storageBackend, cfg.Storage.AllowedMIMEs, cfg.Storage.MaxFileSize,
	)
	catalogRepo := productrepo.NewCatalogPG(pgPool)
	catalogSvc := productservice.NewCatalog(catalogRepo, productRepo)
	ootdAllowedMIMEs := func() map[string]string {
		extMap := map[string]string{"image/jpeg": "jpg", "image/png": "png", "image/webp": "webp"}
		m := make(map[string]string, len(cfg.Storage.AllowedMIMEs))
		for _, mime := range cfg.Storage.AllowedMIMEs {
			if ext, ok := extMap[mime]; ok {
				m[mime] = ext
			}
		}
		return m
	}()
	ootdSvc := ootdservice.New(ootdrepo.NewOOTDPg(pgPool), storageBackend, ootdAllowedMIMEs, cfg.Storage.MaxFileSize)
	ootdHandler := ootdhandler.New(ootdSvc)
	customerAddrSvc := customeraddrservice.New(customerAddrRepo, locSvc)
	wishlistSvc := wishlistservice.New(wishlistRepo, productRepo)
	cartSvc := cartservice.New(cartRepo, variantRepo)
	storeSvc := storeservice.New(storerepo.NewStorePG(pgPool), goongClient)

	// ── Sprint 3 repos ──
	orderRepoSvc := orderrepo.NewOrderPG(pgPool)
	subOrderRepo := orderrepo.NewSubOrderPG(pgPool)
	orderItemRepo := orderrepo.NewOrderItemPG(pgPool)
	paymentRepo := paymentrepo.NewPaymentPG(pgPool)

	// ── shipping provider ──

	shippingProvider, err := provider.NewFromConfig(
		provider.Config{Provider: cfg.Shipping.Provider},
		brandRepo,
		&provider.GoshipDeps{
			Client:     goshipClient,
			PickupRepo: addressRepo,
			Defaults: weight.Defaults{
				WeightG:  cfg.Goship.DefaultItemWeightG,
				LengthCM: cfg.Goship.DefaultLengthCM,
				WidthCM:  cfg.Goship.DefaultWidthCM,
				HeightCM: cfg.Goship.DefaultHeightCM,
			},
		},
	)
	if err != nil {
		log.Fatalf("shipping provider: %v", err)
	}

	// ── PayOS client ──
	payosClient, err := payos.NewFromConfig(payos.Config{
		Mode:        cfg.Payos.Mode,
		ClientID:    cfg.Payos.ClientID,
		APIKey:      cfg.Payos.APIKey,
		ChecksumKey: cfg.Payos.ChecksumKey,
		BaseURL:     cfg.Payos.BaseURL,
	})
	if err != nil {
		log.Fatalf("payos: %v", err)
	}

	// ── Sprint 3 services ──
	promoRepo := promorepo.NewPromoPG(pgPool)
	promoSvc := promoservice.New(promoRepo)
	checkoutSvc := orderservice.NewCheckoutService(cartRepo, customerAddrRepo, shippingProvider, promoSvc)
	orderSvc := orderservice.NewOrderService(
		pgPool,
		orderRepoSvc, subOrderRepo, orderItemRepo,
		paymentRepo, variantRepo,
		customerAddrRepo, userRepo,
		shippingProvider, payosClient, promoSvc,
		orderservice.Config{
			ReservationTimeout: time.Duration(cfg.Reservation.TimeoutMinutes) * time.Minute,
			PayosReturnURL:     cfg.Payos.ReturnURL,
			PayosCancelURL:     cfg.Payos.CancelURL,
		},
	)
	webhookSvc := paymentservice.NewWebhookService(
		pgPool, paymentRepo, orderRepoSvc, subOrderRepo, orderItemRepo, variantRepo, payosClient,
	)

	// ── Fulfillment + shipping webhook services ──
	fulfillmentSvc := orderservice.NewFulfillmentService(
		pgPool,
		orderRepoSvc, subOrderRepo, orderItemRepo, goshipClient, addressRepo,
		weight.Defaults{
			WeightG:  cfg.Goship.DefaultItemWeightG,
			LengthCM: cfg.Goship.DefaultLengthCM,
			WidthCM:  cfg.Goship.DefaultWidthCM,
			HeightCM: cfg.Goship.DefaultHeightCM,
		},
	)
	shippingWebhookSvc := orderservice.NewShippingWebhookService(
		pgPool, subOrderRepo, orderRepoSvc, orderItemRepo, paymentRepo, variantRepo,
	)
	goshipMockMode := cfg.Goship.Mode == "" || cfg.Goship.Mode == "mock"
	brandFulfilHandler := orderhandler.NewBrandFulfillmentHandler(fulfillmentSvc)
	shippingWebhookHandler := orderhandler.NewShippingWebhookHandler(shippingWebhookSvc, goshipClient, goshipMockMode)

	// ── Sprint 3 handlers ──
	orderH := orderhandler.New(checkoutSvc, orderSvc)
	paymentH := paymenthandler.New(webhookSvc, payosClient, cfg.Payos.Mode == "mock", cfg.Payos.ReturnURL, cfg.Payos.CancelURL)

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
	catalogHandler := producthandler.NewCatalogHandler(catalogSvc, categoryRepo, styleTagRepo, brandRepo)
	brandsPublicHandler := brandhandler.NewBrandsPublicHandler(brandSvc)
	customerAddrHandler := customeraddrhandler.New(customerAddrSvc)
	wishlistHandler := wishlisthandler.New(wishlistSvc)
	cartHandler := carthandler.New(cartSvc)
	styleProfileSvc := styleprofileservice.New(styleprofilerepo.NewStyleProfilePG(pgPool))
	styleProfileHandler := styleprofilehandler.New(styleProfileSvc)

	recSvc := recommendationservice.New(
		recommendationrepo.NewCandidatePG(pgPool),
		recommendationrepo.NewSignalPG(pgPool),
		styleProfileSvc,
		recommendationservice.NewRedisCache(rdb),
		recommendationservice.Config{
			DefaultLimit:  cfg.Recommendation.DefaultLimit,
			MaxLimit:      cfg.Recommendation.MaxLimit,
			CandidatePool: cfg.Recommendation.CandidatePool,
		},
	)
	recommendationHandler := recommendationhandler.New(recSvc)

	// dayStamp is captured once at startup. It only affects the empty-closet
	// daily refresh of the wardrobe signature, so a long-running process keeps
	// the start-date stamp until restart (acceptable; processes restart on deploy).
	dayStamp := time.Now().UTC().Format("20060102")
	wardrobeSvc := wardrobeservice.New(
		wardroberepo.NewClosetPG(pgPool),
		wardroberepo.NewSnapshotPG(pgPool),
		styleProfileSvc,
		wardrobeservice.NewCatalogRetriever(catalogSvc),
		llmClient,
		wardrobeservice.Config{MaxOutfits: 5, ToBuyPerOutfit: 2, DayStamp: dayStamp},
	)
	wardrobeHandler := wardrobehandler.New(wardrobeSvc)

	styleProfileSvc.SetOnSaved(func(ctx context.Context, userID uuid.UUID) {
		if err := recSvc.Invalidate(ctx, userID); err != nil {
			log.Printf("recommendation: invalidate after profile save failed for %s: %v", userID, err)
		}
	})

	// ── router ──
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(gin.Logger())
	r.Use(httpmw.CORS(cfg.CORS.AllowedOrigins))

	// Limit multipart form memory so large uploads spill to temp files rather
	// than being held entirely in RAM. Per-file size enforcement stays in the
	// service layer.
	multipartLimit := cfg.Storage.MaxFileSize
	if multipartLimit <= 0 {
		multipartLimit = 4 << 20 // 4 MiB default
	}
	r.MaxMultipartMemory = multipartLimit

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
	orderhandler.MountBrand(brandGroup, brandFulfilHandler)

	producthandler.MountCatalog(v1, catalogHandler)
	brandhandler.MountBrandsPublic(v1, brandsPublicHandler)
	storehandler.MountStoresPublic(v1, storehandler.NewHandler(storeSvc))
	reviewhandler.MountReviewsPublic(v1, reviewHandler)
	ootdhandler.MountOOTDPublic(v1, ootdHandler)

	customerGroup := v1.Group("/me",
		middleware.RequireAuth(jwtIssuer),
		middleware.RequireRole(authdomain.RoleCustomer),
	)
	customeraddrhandler.Mount(customerGroup, customerAddrHandler)
	wishlisthandler.Mount(customerGroup, wishlistHandler)
	carthandler.Mount(customerGroup, cartHandler)
	orderhandler.Mount(customerGroup, orderH)
	styleprofilehandler.Mount(customerGroup, styleProfileHandler)
	recommendationhandler.Mount(customerGroup, recommendationHandler)
	wardrobehandler.Mount(customerGroup, wardrobeHandler)

	reviewsAuthed := v1.Group("", middleware.RequireAuth(jwtIssuer))
	reviewhandler.MountReviewsAuthed(reviewsAuthed, reviewHandler)
	ootdhandler.MountOOTDAuthed(reviewsAuthed, ootdHandler)
	followhandler.MountFollowAuthed(reviewsAuthed, followHandler)
	blockhandler.MountBlockAuthed(reviewsAuthed, blockHandler)

	adminGroup := v1.Group("/admin",
		middleware.RequireAuth(jwtIssuer),
		middleware.RequireRole(authdomain.RoleAdmin),
	)
	promohandler.MountAdmin(adminGroup, promohandler.New(promoSvc))

	location.RegisterRoutes(v1, location.NewHandler(locSvc))
	paymenthandler.MountPublic(v1, paymentH)
	orderhandler.MountShippingPublic(v1, shippingWebhookHandler)

	if cfg.Payos.Mode == "mock" {
		devGroup := r.Group("/dev")
		paymenthandler.MountDev(devGroup, paymentH)
	}
	if cfg.Goship.Mode == "" || cfg.Goship.Mode == "mock" {
		devGroup := r.Group("/dev")
		orderhandler.MountShippingDev(devGroup, shippingWebhookHandler)
	}

	// ── cleanup job ──
	cleanupJob := jobsmod.NewReservationCleanupJob(
		pgPool, orderRepoSvc, subOrderRepo, orderItemRepo,
		paymentRepo, variantRepo, cfg.Reservation.TimeoutMinutes,
	)
	go cleanupJob.Run(ctx, cfg.Reservation.CleanupInterval)

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
