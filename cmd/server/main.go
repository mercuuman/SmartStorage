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
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️ .env not found, using system env")
	}

	db, err := database.NewPostgres()
	if err != nil {
		log.Fatal("❌ DB connection failed:", err)
	}
	defer db.Close()

	r := gin.Default()
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:5173"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	authRepo := auth.NewRepository(db)
	authService := auth.NewService(authRepo)
	authHandler := auth.NewHandler(authService)
	compressionManager := compression.NewManager()

	filesRepo := files.NewRepository(db)
	storage := files.NewLocalStorage("./uploads")
	filesService := files.NewService(filesRepo, storage, compressionManager)
	filesHandler := files.NewHandler(filesService)

	analyticsRepo := analytics.NewRepository(db)
	analyticsService := analytics.NewService(analyticsRepo)
	analyticsHandler := analytics.NewHandler(analyticsService)

	api := r.Group("/api")
	authGroup := api.Group("/auth")
	{
		authGroup.POST("/register", authHandler.Register)
		authGroup.POST("/login", authHandler.Login)
	}

	protected := api.Group("/")
	protected.Use(middleware.AuthMiddleware())
	{
		protected.GET("/profile", func(c *gin.Context) {
			userID := c.GetString("userID")
			user, err := authService.GetUser(c.Request.Context(), userID)
			if err != nil {
				c.JSON(500, gin.H{"error": err.Error()})
				return
			}
			c.JSON(200, user)
		})

		// FILES
		filesGroup := protected.Group("/files")
		{
			filesGroup.POST("/upload", filesHandler.Upload)
			filesGroup.GET("/", filesHandler.List)
			filesGroup.GET("/:id/download", filesHandler.Download)
			filesGroup.DELETE("/:id", filesHandler.Delete) // теперь → в Корзину
			filesGroup.GET("/:id", filesHandler.GetFileDetails)
			filesGroup.GET("/:id/versions", filesHandler.GetVersionHistory)
			filesGroup.POST("/:id/restore/:version", filesHandler.RestoreVersion)
		}

		// FOLDERS
		foldersGroup := protected.Group("/folders")
		{
			foldersGroup.POST("/", filesHandler.CreateFolder)
			foldersGroup.GET("/", filesHandler.ListFolders)
			foldersGroup.GET("/:id", filesHandler.GetFolder)
			foldersGroup.DELETE("/:id", filesHandler.DeleteFolder) // → в Корзину
		}

		// TRASH
		trashGroup := protected.Group("/trash")
		{
			trashGroup.GET("/", filesHandler.GetTrashContents)
			trashGroup.DELETE("/", filesHandler.EmptyTrash)
			trashGroup.POST("/restore", filesHandler.RestoreFromTrash)
		}

		// ANALYTICS
		analytics := protected.Group("/analytics")
		{
			analytics.GET("/system", analyticsHandler.GetSystemStats)
			analytics.GET("/user", analyticsHandler.GetUserStats)
			analytics.GET("/compression", analyticsHandler.GetCompressionStats)
		}
	}

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "OK"})
	})

	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("🚀 Server running on :%s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatal("❌ Server failed:", err)
	}
}
