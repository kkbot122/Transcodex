package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kisna/transcodex/pkg/postgres"
	redisclient "github.com/kisna/transcodex/pkg/redis"
	"github.com/kisna/transcodex/pkg/storage"
	"github.com/redis/go-redis/v9"
)

type Server struct {
	config  Config
	db      *pgxpool.Pool
	redis   *redis.Client
	storage *storage.Client
	router  *gin.Engine
}

func New(ctx context.Context, config Config) (*Server, error) {
	db, err := postgres.NewPool(ctx, config.DatabaseURL)
	if err != nil {
		return nil, err
	}

	redisConn, err := redisclient.NewClient(ctx, config.RedisURL)
	if err != nil {
		db.Close()
		return nil, err
	}

	storageClient, err := storage.NewClient(ctx, storage.Config{
		Endpoint:  config.StorageEndpoint,
		AccessKey: config.StorageAccessKey,
		SecretKey: config.StorageSecretKey,
		Bucket:    config.StorageBucket,
	})
	if err != nil {
		_ = redisConn.Close()
		db.Close()
		return nil, err
	}

	server := &Server{
		config:  config,
		db:      db,
		redis:   redisConn,
		storage: storageClient,
	}
	server.router = server.buildRouter()

	return server, nil
}

func (s *Server) Router() http.Handler {
	return s.router
}

func (s *Server) Close() {
	if s.redis != nil {
		_ = s.redis.Close()
	}
	if s.db != nil {
		s.db.Close()
	}
}

func (s *Server) buildRouter() *gin.Engine {
	router := gin.New()
	router.Use(requestLogger(), cors.New(cors.Config{
		AllowOrigins: s.config.CORSAllowedOrigins,
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodOptions},
		AllowHeaders: []string{"Content-Type", "Authorization"},
	}), gin.CustomRecovery(recoveryHandler))

	router.GET("/healthz", s.healthz)
	router.POST("/uploads", s.upload)
	router.GET("/jobs/:id", s.getJob)
	router.GET("/jobs/:id/outputs", s.getJobOutputs)
	router.GET("/internal/stats", s.getStats)
	router.GET("/internal/jobs", s.getInternalJobs)
	router.GET("/internal/workers", s.getWorkers)
	router.GET("/internal/stats/stream", s.streamStats)

	return router
}

func (s *Server) healthz(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func requestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		slog.Info("http request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"latency", time.Since(start),
		)
	}
}

func recoveryHandler(c *gin.Context, recovered any) {
	slog.Error("panic recovered", "panic", recovered, "path", c.Request.URL.Path, "method", c.Request.Method)
	c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
}

func respondError(c *gin.Context, status int, message string) {
	c.AbortWithStatusJSON(status, gin.H{"error": message})
}
