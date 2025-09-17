package server

import (
	"backend/internal/api/rest"
	"backend/internal/auth"
	ratelimit "backend/internal/ratelimiter"
	"backend/internal/storage"
	"backend/pkg/logger"
	"net/http"
	"os"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	graphqllayer "backend/internal/api/graphql"
	"backend/internal/api/graphql/generated"

	"github.com/99designs/gqlgen/graphql/handler"
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

	rl := ratelimit.NewRateLimiter(s.rdb, 2, time.Second)
	r.Use(rl.Middleware())

	r.GET("/", s.HelloWorldHandler)
	r.GET("/health", s.healthHandler)

	jwtSecret := os.Getenv("JWT_SECRET")

	// GraphQL handlers (POST for queries/mutations)
	{
		execSchema := handler.NewDefaultServer(
			generated.NewExecutableSchema(
				generated.Config{Resolvers: &graphqllayer.Resolver{DB: s.db.DB()}},
			),
		)
		r.POST("/api/v1/graphql", auth.RequireAuth(jwtSecret), func(c *gin.Context) {
			execSchema.ServeHTTP(c.Writer, c.Request)
		})
		// Optional: playground (remove in prod)
		// r.GET("/playground", func(c *gin.Context) {
		// 	playground.Handler("GraphQL Playground", "/api/v1/graphql").ServeHTTP(c.Writer, c.Request)
		// })
	}

	api := r.Group("/api/v1")
	{
		rest.RegisterAuthRoutes(api, s.db)

		st, err := storage.NewMinioStorage()
		if err != nil {
			logger.L.Fatal("storage init failed", zap.Error(err))
		}

		// jwtSecret := os.Getenv("JWT_SECRET")
		rest.RegisterAdminRoutes(api, s.db.DB(), jwtSecret)
		rest.RegisterQuotaRoutes(api, s.db.DB(), jwtSecret)
		rest.RegisterSearchRoutes(api, s.db.DB(), jwtSecret, s.cache)
		rest.RegisterFolderRoutes(api, s.db.DB(), jwtSecret)
		rest.RegisterShareRoutes(api, s.db.DB(), st, jwtSecret, s.cache)
		rest.RegisterUploadRoutes(api, s.db.DB(), st, jwtSecret)
		rest.RegisterFileRoutes(api, s.db.DB(), st, jwtSecret, s.cache)
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
