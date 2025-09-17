package server

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	_ "github.com/joho/godotenv/autoload"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"backend/internal/cache"
	"backend/internal/database"
	"backend/pkg/logger"
)

type Server struct {
	port  int
	db    database.Service
	rdb   *redis.Client
	cache cache.Cache
}

func NewServer() *http.Server {
	port, _ := strconv.Atoi(os.Getenv("PORT"))
	rdb, err := database.InitRedis()
	if err != nil {
		logger.L.Fatal("failed to init redis", zap.Error(err))
	}
	redisCache := cache.NewRedisCache(rdb)
	db := database.New()
	NewServer := &Server{
		port:  port,
		db:    db,
		rdb:   rdb,
		cache: redisCache,
	}

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", NewServer.port),
		Handler:      NewServer.RegisterRoutes(),
		IdleTimeout:  time.Minute,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	return server
}
