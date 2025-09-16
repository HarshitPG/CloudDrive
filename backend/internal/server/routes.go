package server

import (
	"backend/internal/api/rest"
	"backend/internal/storage"
	"backend/pkg/logger"
	"net/http"
	"os"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func (s *Server) RegisterRoutes() http.Handler {
	r := gin.New()
	gin.SetMode(gin.ReleaseMode)
	_ = logger.Init()

	r.Use(gin.Recovery())
	r.Use(gin.Logger())

	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:5173"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowHeaders:     []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true,
	}))

	r.GET("/", s.HelloWorldHandler)

	r.GET("/health", s.healthHandler)
	api := r.Group("/api/v1")
	{
		rest.RegisterAuthRoutes(api, s.db)

		st, err := storage.NewMinioStorage()
		if err != nil {
			logger.L.Fatal("storage init failed", zap.Error(err))
		}

		jwtSecret := os.Getenv("JWT_SECRET")
		rest.RegisterFolderRoutes(api, s.db.DB(), jwtSecret)
		rest.RegisterUploadRoutes(api, s.db.DB(), st, jwtSecret)
		rest.RegisterFileRoutes(api, s.db.DB(), st, jwtSecret)
	}

	return r
}

func (s *Server) HelloWorldHandler(c *gin.Context) {
	resp := make(map[string]string)
	resp["message"] = "Hello World"

	c.JSON(http.StatusOK, resp)
}

func (s *Server) healthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, s.db.Health())
}
