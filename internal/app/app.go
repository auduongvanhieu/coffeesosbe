// Package app wires configuration, database, realtime hub and HTTP routes
// into one server. Route groups mirror the four access levels:
//
//	/api/v1/platform  super-admin (manage brands)
//	/api/v1/admin     brand owner / store manager (manage the brand)
//	/api/v1/pos       staff terminals
//	/api/v1/app       customers (public for now, phone+OTP later)
package app

import (
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"coffeesos/internal/auth"
	"coffeesos/internal/brand"
	"coffeesos/internal/config"
	"coffeesos/internal/db"
	"coffeesos/internal/menu"
	"coffeesos/internal/order"
	"coffeesos/internal/realtime"
	"coffeesos/internal/storage"
	"coffeesos/internal/tenant"
)

type App struct {
	Engine *gin.Engine
	Hub    *realtime.Hub
}

func New(cfg *config.Config, pool *pgxpool.Pool, log *slog.Logger) *App {
	if !cfg.IsDev() {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	// Only these networks may set X-Forwarded-For (nginx reaches the container via the docker bridge).
	if err := r.SetTrustedProxies(cfg.TrustedProxies); err != nil {
		log.Warn("invalid TRUSTED_PROXIES, trusting none", "err", err)
		_ = r.SetTrustedProxies(nil)
	}
	r.Use(gin.Recovery(), requestLogger(log), corsMiddleware(cfg))

	queries := db.New(pool)
	issuer := auth.NewTokenIssuer(cfg.JWTSecret, cfg.JWTTTL)
	hub := realtime.NewHub(log, originChecker(cfg))

	authH := auth.NewHandler(queries, issuer)
	menuH := menu.NewHandler(menu.NewService(queries, hub), queries)
	brandH := brand.NewHandler(queries)
	store := storage.New(storage.Config{
		AccountID: cfg.R2AccountID, AccessKeyID: cfg.R2AccessKeyID, SecretAccessKey: cfg.R2SecretAccessKey,
		Bucket: cfg.R2Bucket, PublicBaseURL: cfg.R2PublicBaseURL,
	})
	if !store.Enabled() {
		log.Warn("R2 storage not configured; POST /admin/uploads will return 503 (set R2_* env vars)")
	}
	uploadH := storage.NewHandler(store)
	orderH := order.NewHandler(order.NewService(pool, queries, hub), queries)

	r.GET("/healthz", func(c *gin.Context) {
		ctx, cancel := contextWithTimeout(c, 2*time.Second)
		defer cancel()
		if err := pool.Ping(ctx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "degraded", "db": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	v1 := r.Group("/api/v1")

	// Auth
	v1.POST("/auth/login", authH.Login)
	v1.POST("/auth/pin-login", authH.PinLogin)
	v1.GET("/auth/me", auth.RequireAuth(issuer), authH.Me)

	// Realtime
	r.GET("/ws", auth.RequireAuth(issuer), hub.ServeWS)

	// Platform
	platform := v1.Group("/platform", auth.RequireAuth(issuer), auth.RequireLevel(tenant.LevelPlatformAdmin))
	platform.GET("/brands", brandH.ListBrands)
	platform.POST("/brands", brandH.CreateBrand)
	platform.POST("/brands/:brandId/stores", brandH.CreateStoreForBrand)
	platform.POST("/brands/:brandId/users", brandH.CreateUserForBrand)

	// Brand admin
	admin := v1.Group("/admin", auth.RequireAuth(issuer), auth.RequireLevel(tenant.LevelStoreManager))
	admin.GET("/brand", brandH.CurrentBrand)
	admin.PUT("/brand", auth.RequireLevel(tenant.LevelBrandOwner), brandH.UpdateBrand)
	admin.GET("/stats", brandH.Stats)
	admin.GET("/stores", brandH.ListStores)
	admin.POST("/stores", auth.RequireLevel(tenant.LevelBrandOwner), brandH.CreateStore)
	admin.GET("/users", auth.RequireLevel(tenant.LevelBrandOwner), brandH.ListUsers)
	admin.POST("/users", brandH.CreateUser)
	admin.PUT("/users/:id/pin", authH.SetPin)
	admin.POST("/uploads", uploadH.Upload)
	admin.GET("/menu/categories", menuH.ListCategories)
	admin.POST("/menu/categories", auth.RequireLevel(tenant.LevelBrandOwner), menuH.CreateCategory)
	admin.PATCH("/menu/categories/:id", auth.RequireLevel(tenant.LevelBrandOwner), menuH.UpdateCategory)
	admin.GET("/menu/items", menuH.ListItems)
	admin.POST("/menu/items", auth.RequireLevel(tenant.LevelBrandOwner), menuH.CreateItem)
	admin.PUT("/menu/items/:id", auth.RequireLevel(tenant.LevelBrandOwner), menuH.UpdateItem)
	admin.PATCH("/menu/items/:id/availability", menuH.SetAvailability)

	// POS
	pos := v1.Group("/pos", auth.RequireAuth(issuer), auth.RequireLevel(tenant.LevelStaff))
	pos.GET("/menu", menuH.StoreMenu)
	pos.PATCH("/menu/items/:id/availability", orderH.SetAvailability)
	pos.GET("/store", orderH.Store)
	pos.GET("/customers/lookup", orderH.LookupCustomer)
	pos.POST("/customers", orderH.CreateCustomer)
	pos.GET("/promotions/:code", orderH.Promotion)
	pos.GET("/orders", orderH.List)
	pos.GET("/orders/summary", orderH.Summary)
	pos.POST("/orders", orderH.Create)
	pos.GET("/orders/:id", orderH.Get)
	pos.PUT("/orders/:id", orderH.Replace)
	pos.POST("/orders/:id/pay", orderH.Pay)
	pos.PATCH("/orders/:id/status", orderH.SetStatus)

	// Customer app (public endpoints)
	customer := v1.Group("/app")
	customer.GET("/stores/:storeId/menu", menuH.PublicStoreMenu)
	customer.POST("/stores/:storeId/orders", orderH.CreateFromApp)

	return &App{Engine: r, Hub: hub}
}

func corsMiddleware(cfg *config.Config) gin.HandlerFunc {
	conf := cors.Config{
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Authorization", "Content-Type", tenant.HeaderBrandID, tenant.HeaderStoreID},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: false,
		MaxAge:           12 * time.Hour,
	}
	if slices.Contains(cfg.CORSOrigins, "*") {
		conf.AllowAllOrigins = true
	} else {
		conf.AllowOrigins = cfg.CORSOrigins
	}
	return cors.New(conf)
}

func originChecker(cfg *config.Config) func(r *http.Request) bool {
	allowAll := slices.Contains(cfg.CORSOrigins, "*")
	return func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if allowAll || origin == "" {
			return true
		}
		return slices.Contains(cfg.CORSOrigins, origin)
	}
}

func requestLogger(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		attrs := []any{
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"duration", time.Since(start).Round(time.Millisecond).String(),
			"ip", c.ClientIP(),
		}
		if len(c.Errors) > 0 {
			attrs = append(attrs, "err", c.Errors.String())
			log.Error("request", attrs...)
			return
		}
		if c.Writer.Status() >= 500 {
			log.Error("request", attrs...)
		} else {
			log.Info("request", attrs...)
		}
	}
}
