package main

import (
	"diplom/internal/analytics"
	"diplom/internal/auth"
	"diplom/internal/compression"
	"diplom/internal/database"
	"diplom/internal/files"
	"diplom/internal/middleware"
	"log"
	"os"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	// 1. Load env
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️ .env not found, using system env")
	}

	// 2. DB init
	db, err := database.NewPostgres()
	if err != nil {
		log.Fatal("❌ DB connection failed:", err)
	}
	defer db.Close()

	// 3. Router
	r := gin.Default()

	// CORS
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:5173"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// =========================
	// AUTH MODULE
	// =========================
	authRepo := auth.NewRepository(db)
	authService := auth.NewService(authRepo)
	authHandler := auth.NewHandler(authService)
	compressionManager := compression.NewManager()

	// =========================
	// FILES MODULE (NEW ARCH)
	// =========================
	filesRepo := files.NewRepository(db)

	// storage теперь часть service abstraction
	storage := files.NewLocalStorage("./uploads")

	filesService := files.NewService(filesRepo, storage, compressionManager)
	filesHandler := files.NewHandler(filesService)

	// ANALYTICS MODULE
	analyticsRepo := analytics.NewRepository(db)
	analyticsService := analytics.NewService(analyticsRepo)
	analyticsHandler := analytics.NewHandler(analyticsService)

	// =========================
	// API ROUTES
	// =========================
	api := r.Group("/api")

	// ---- AUTH ----
	authGroup := api.Group("/auth")
	{
		authGroup.POST("/register", authHandler.Register)
		authGroup.POST("/login", authHandler.Login)
	}

	// ---- PROTECTED ----
	protected := api.Group("/")
	protected.Use(middleware.AuthMiddleware())
	{
		// profile test
		protected.GET("/profile", func(c *gin.Context) {
			userID := c.GetString("userID")
			c.JSON(200, gin.H{
				"user_id": userID,
			})
		})

		// ---- FILES ----
		filesGroup := protected.Group("/files")
		{
			filesGroup.POST("/upload", filesHandler.Upload)
			filesGroup.GET("/", filesHandler.List)
			filesGroup.GET("/:id/download", filesHandler.Download)
			filesGroup.DELETE("/:id", filesHandler.Delete)
			filesGroup.GET("/:id", filesHandler.GetFileDetails)
			filesGroup.GET("/:id/versions", filesHandler.GetVersionHistory)
			filesGroup.POST("/:id/restore/:version", filesHandler.RestoreVersion)
		}
		analytics := protected.Group("/analytics")
		{
			analytics.GET("/system", analyticsHandler.GetSystemStats)
			analytics.GET("/user", analyticsHandler.GetUserStats)
			analytics.GET("/compression", analyticsHandler.GetCompressionStats)
		}
	}

	// =========================
	// HEALTHCHECK
	// =========================
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "OK",
		})
	})

	// =========================
	// START SERVER
	// =========================
	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("🚀 Server running on :%s", port)

	if err := r.Run(":" + port); err != nil {
		log.Fatal("❌ Server failed:", err)
	}
}
