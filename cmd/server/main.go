package main

import (
	"diplom/internal/auth"
	"log"
	"os"
	"strings"

	"diplom/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️ .env not found, using env vars")
	}

	db, err := database.NewPostgres()
	if err != nil {
		log.Fatal("❌ DB connection failed:", err)
	}

	defer db.Close()

	r := gin.Default()

	authRepo := auth.NewRepository(db)
	authService := auth.NewService(authRepo)
	authHandler := auth.NewHandler(authService)

	api := r.Group("/api")
	{
		authGroup := api.Group("/auth")
		{
			authGroup.POST("/register", authHandler.Register)
			authGroup.POST("/login", authHandler.Login)
		}
	}

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "OK"})
	})

	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "8080"
	}
	if !strings.HasPrefix(port, ":") {
		port = ":" + port
	}

	log.Printf("🚀 Server starting on %s", port)
	if err := r.Run(port); err != nil {
		log.Fatal("❌ Server failed to start:", err)
	}
}
