package main

import (
	"database/sql"
	"log"
	"net"

	"mangahub/internal/grpc"
	pb "mangahub/proto"

	grpcServer "google.golang.org/grpc"
	_ "modernc.org/sqlite"
)

func main() {
	// Open database 
	db, err := sql.Open("sqlite", "./data/mangahub.db")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Test connection
	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	log.Println("✓ Connected to database")

	// Create TCP listener
	lis, err := net.Listen("tcp", ":9092")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	// Create gRPC server
	grpcSrv := grpcServer.NewServer()

	// Register manga service
	mangaService := grpc.NewMangaServiceServer(db)
	pb.RegisterMangaServiceServer(grpcSrv, mangaService)

	log.Println("🚀 gRPC server listening on :9092")
	log.Println("📡 Manga service registered")

	// Start serving
	if err := grpcSrv.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
