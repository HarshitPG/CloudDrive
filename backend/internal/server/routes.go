package server

import (
	"backend/internal/api/rest"
	"backend/internal/auth"
	"backend/internal/llmclient"
	"backend/internal/notifications"
	ratelimit "backend/internal/ratelimiter"
	"backend/internal/storage"
	"backend/pkg/logger"
	"context"
	"net/http"
	"os"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	graphqllayer "backend/internal/api/graphql"
	"backend/internal/api/graphql/generated"

	"github.com/99designs/gqlgen/graphql/handler"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
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

	rl := ratelimit.NewRateLimiter(s.rdb, 20, time.Second)
	r.Use(rl.Middleware())

	r.GET("/", s.HelloWorldHandler)
	r.GET("/health", s.healthHandler)

	// Swagger UI (served at /swagger/index.html)
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	jwtSecret := os.Getenv("JWT_SECRET")

	// GraphQL handlers (POST for queries/mutations)
	{
		execSchema := handler.NewDefaultServer(
			generated.NewExecutableSchema(
				generated.Config{Resolvers: &graphqllayer.Resolver{DB: s.db.DB()}},
			),
		)
		//	@Summary		GraphQL endpoint
		//	@Description	Send GraphQL queries and mutations
		//	@Tags			graphql
		//	@Accept			json
		//	@Produce		json
		//	@Security		BearerAuth
		//	@Router			/api/v1/graphql [post]
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

		var llmClient *llmclient.Client
		llmAddr := os.Getenv("LLM_GRPC_ADDR")
		llmToken := os.Getenv("LLM_GRPC_TOKEN")

		if llmAddr != "" {
			logger.L.Info("Init LLM gRPC Client", zap.String("address", llmAddr))
			const attempts = 3
			for i := 1; i <= attempts; i++ {
				c, err := llmclient.NewClient(llmclient.Config{
					Address: llmAddr,
					Token:   llmToken,
					Timeout: 30 * time.Second,
				})
				if err == nil {
					llmClient = c
					logger.L.Info("LLM gRPC client initialized successfully")
					break
				}
				logger.L.Warn("Failed to initialize LLM client, will retry", zap.Int("attempt", i), zap.Int("max_attempts", attempts), zap.Error(err))
				time.Sleep(3 * time.Second)
			}
			if llmClient == nil {
				logger.L.Warn("Failed to initialize LLM client after retries (summary/chat features disabled)")
			}
		} else {
			logger.L.Warn("LLM_GRPC_ADDR not set, summary/chat features disabled")
		}
		//	@Summary		WebSocket for download updates
		//	@Description	Connect via WebSocket to receive download count updates. Optional query param fileId to filter.
		//	@Tags			websocket
		//	@Param			fileId	query		string	false	"Filter by file ID"
		//	@Success		101		{string}	string	"Switching Protocols"
		//	@Router			/api/v1/ws/downloads [get]
		api.GET("/ws/downloads", auth.RequireAuth(jwtSecret), func(c *gin.Context) {
			notifications.ServeWS(s.hub)(c.Writer, c.Request)
		})
		// jwtSecret := os.Getenv("JWT_SECRET")
		rest.RegisterAdminRoutes(api, s.db.DB(), jwtSecret)
		rest.RegisterQuotaRoutes(api, s.db.DB(), jwtSecret)
		rest.RegisterSearchRoutes(api, s.db.DB(), jwtSecret, s.cache)
		rest.RegisterShareRoutes(api, s.db.DB(), st, jwtSecret, s.cache)
		rest.RegisterUploadRoutes(api, s.db.DB(), st, jwtSecret, s.cache)
		rest.RegisterFolderRoutes(api, s.db.DB(), jwtSecret, s.cache, st)
		rest.RegisterFileRoutes(api, s.db.DB(), st, jwtSecret, s.cache, func(ctx context.Context, fileID string, count int64) error {
			return notifications.PublishDownload(ctx, s.rdb, fileID, count)
		}, llmClient)
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
