package main

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"mangahub/internal/auth"
	"mangahub/internal/config"
	pb "mangahub/proto"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// healthHandler reports the operational status of all unified backend protocols
func healthHandler(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		catalogStatus := "unknown"
		if grpcCatalogClient != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()
			_, err := grpcCatalogClient.SearchManga(ctx, &pb.SearchMangaRequest{Query: ""})
			if err != nil {
				catalogStatus = "unreachable"
			} else {
				catalogStatus = "connected"
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"status":    "healthy",
			"timestamp": time.Now().Unix(),
			"services": gin.H{
				"http":         "running (:" + cfg.HTTPPort + ")",
				"tcp":          "running (:" + cfg.TCPPort + ")",
				"udp":          "running (:" + cfg.UDPPort + ")",
				"websocket":    "/ws",
				"grpc_catalog": catalogStatus,
			},
		})
	}
}

// setupRouter configures Gin routes for both REST and WebSocket endpoints
func setupRouter(cfg *config.Config) *gin.Engine {
	router := gin.Default()
	router.Use(cors.Default())

	// Health check
	router.GET("/api/health", healthHandler(cfg))

	// Public Auth Endpoints
	authGroup := router.Group("/api/auth")
	{
		authGroup.POST("/register", registerHandler)
		authGroup.POST("/login", loginHandler)
	}
	// Backwards-compatible aliases
	router.POST("/auth/register", registerHandler)
	router.POST("/auth/login", loginHandler)

	// Admin / Demo Release Endpoint (HTTP -> gRPC update -> UDP alert)
	router.POST("/api/admin/manga/:id/release", adminReleaseChapterHandler)
	router.POST("/admin/manga/:id/release", adminReleaseChapterHandler)

	// Protected Endpoints
	api := router.Group("/api")
	api.Use(auth.Middleware())
	{
		api.GET("/manga", getMangaHandler)
		api.GET("/manga/:id", getMangaDetailHandler)
		api.GET("/library", getLibraryHandler)
		api.POST("/library", addToLibraryHandler)
		api.PUT("/library/:mangaId/progress", updateProgressHandler)
	}

	// Backwards-compatible aliases
	protectedLegacy := router.Group("/")
	protectedLegacy.Use(auth.Middleware())
	{
		protectedLegacy.GET("/manga", getMangaHandler)
		protectedLegacy.GET("/manga/:id", getMangaDetailHandler)
		protectedLegacy.GET("/users/library", getLibraryHandler)
		protectedLegacy.POST("/users/library", addToLibraryHandler)
		protectedLegacy.PUT("/users/progress", updateProgressLegacyHandler)
	}

	// WebSocket discussion upgrade endpoint
	router.GET("/ws", handleWebSocketUpgrade)

	// Static web assets if present
	if _, err := os.Stat("web"); err == nil {
		router.Static("/web", "./web")
		router.GET("/", func(c *gin.Context) {
			c.File(filepath.Join("web", "search_manga.html"))
		})
	}

	return router
}
