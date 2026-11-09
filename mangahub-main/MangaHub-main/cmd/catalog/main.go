package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net"
	"os"
	"path/filepath"

	"mangahub/internal/config"
	"mangahub/internal/grpc"
	"mangahub/pkg/models"
	pb "mangahub/proto"

	grpcServer "google.golang.org/grpc"
	_ "modernc.org/sqlite"
)

func main() {
	cfg := config.Load()

	// Ensure directory for database exists
	dir := filepath.Dir(cfg.CatalogDBPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, os.ModePerm); err != nil {
			log.Fatalf("[Catalog] Failed to create db dir: %v", err)
		}
	}

	// Open Catalog SQLite database
	db, err := sql.Open("sqlite", cfg.CatalogDBPath)
	if err != nil {
		log.Fatalf("[Catalog] Failed to open database (%s): %v", cfg.CatalogDBPath, err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("[Catalog] Failed to ping database: %v", err)
	}

	log.Printf("[Catalog] Connected to SQLite: %s", cfg.CatalogDBPath)

	// Ensure manga schema exists
	if err := initCatalogSchema(db); err != nil {
		log.Fatalf("[Catalog] Failed to initialize catalog schema: %v", err)
	}

	// Seed manga data if empty
	if err := seedCatalog(db); err != nil {
		log.Printf("[Catalog] Warning: seed warning: %v", err)
	}

	// Listen on gRPC port
	addr := ":" + cfg.GRPCPort
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("[Catalog] Failed to listen on %s: %v", addr, err)
	}

	server := grpcServer.NewServer()
	mangaService := grpc.NewMangaServiceServer(db)
	pb.RegisterMangaServiceServer(server, mangaService)

	log.Printf("==================================================")
	log.Printf("🚀 MANGA CATALOG gRPC SERVICE RUNNING ON %s", addr)
	log.Printf("   Independent process for catalog management")
	log.Printf("==================================================")

	if err := server.Serve(lis); err != nil {
		log.Fatalf("[Catalog] Failed to serve: %v", err)
	}
}

func initCatalogSchema(db *sql.DB) error {
	query := `CREATE TABLE IF NOT EXISTS manga (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		author TEXT,
		genres TEXT,
		status TEXT,
		total_chapters INTEGER,
		description TEXT
	);`
	_, err := db.Exec(query)
	return err
}

func seedCatalog(db *sql.DB) error {
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM manga").Scan(&count); err == nil && count > 0 {
		log.Printf("[Catalog] Manga catalog already has %d entries. Skipping seeding.", count)
		return nil
	}

	seedPaths := []string{
		"data/manga_seed.json",
		"./data/manga_seed.json",
		"../data/manga_seed.json",
		"../../data/manga_seed.json",
	}

	var mangas []models.Manga
	for _, p := range seedPaths {
		if data, err := os.ReadFile(p); err == nil {
			if err := json.Unmarshal(data, &mangas); err == nil && len(mangas) > 0 {
				log.Printf("[Catalog] Loaded %d items from %s", len(mangas), p)
				break
			}
		}
	}

	if len(mangas) == 0 {
		log.Println("[Catalog] No seed file found, skipping initial seed")
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT OR IGNORE INTO manga (id, title, author, genres, status, total_chapters, description)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, m := range mangas {
		m.PreSave()
		if _, err := stmt.Exec(m.ID, m.Title, m.Author, m.GenresString, m.Status, m.TotalChapters, m.Description); err != nil {
			return err
		}
	}

	return tx.Commit()
}
