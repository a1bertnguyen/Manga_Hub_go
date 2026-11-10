package main

import (
	"log"

	"mangahub/internal/config"
	"mangahub/internal/database"
	"mangahub/internal/tcp"
	"mangahub/internal/udp"
	wsHub "mangahub/internal/websocket"
	pb "mangahub/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	globalWSHub       *wsHub.Hub
	grpcCatalogClient pb.MangaServiceClient
	catalogConn       *grpc.ClientConn
)

func main() {
	cfg := config.Load()

	// 1. Initialize SQLite Database for Users & User Library
	if err := database.Initialize(cfg.BackendDBPath); err != nil {
		log.Fatalf("[Backend] Failed to initialize database: %v", err)
	}
	defer database.Close()

	// 2. Initialize gRPC client to Catalog Service
	initCatalogGRPC(cfg.CatalogGRPCAddr)
	if catalogConn != nil {
		defer catalogConn.Close()
	}

	// 3. Start TCP Progress Sync Hub & Listener
	go tcp.GlobalHub.Run()
	go startTCPListener(cfg.TCPPort)

	// 4. Start UDP Notification Hub & Listener
	go udp.GlobalHub.Run()
	go udp.StartUDPListener(":" + cfg.UDPPort)

	// 5. Start WebSocket Discussion Hub
	globalWSHub = wsHub.NewHub()
	go globalWSHub.Run()

	// 6. Initialize Gin Engine (HTTP REST + WebSocket on HTTPPort)
	router := setupRouter(cfg)

	log.Printf("==================================================")
	log.Printf("🚀 MANGAHUB UNIFIED BACKEND RUNNING")
	log.Printf("   HTTP REST & WS : http://localhost:%s", cfg.HTTPPort)
	log.Printf("   TCP Sync Hub   : tcp://localhost:%s", cfg.TCPPort)
	log.Printf("   UDP Notifier   : udp://localhost:%s", cfg.UDPPort)
	log.Printf("   gRPC Catalog   : %s", cfg.CatalogGRPCAddr)
	log.Printf("   Database       : %s", cfg.BackendDBPath)
	log.Printf("==================================================")

	if err := router.Run(":" + cfg.HTTPPort); err != nil {
		log.Fatalf("[Backend] Gin HTTP Server error: %v", err)
	}
}

func initCatalogGRPC(addr string) {
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Printf("[Backend] Warning: gRPC dial to %s returned error: %v", addr, err)
		return
	}
	catalogConn = conn
	grpcCatalogClient = pb.NewMangaServiceClient(conn)
	log.Printf("[Backend] gRPC Catalog Client initialized pointing to %s", addr)
}
