package database

import (
	"database/sql"
	"encoding/json"
	"log"
	"os"
	"path/filepath"

	"mangahub/pkg/models"

	_ "modernc.org/sqlite"
)

// DB is the global database instance
var DB *sql.DB

// Initialize sets up the database connection and creates tables
// This function must be called before any database operations.
// It sets the global database.DB variable.
func Initialize(dbPath string) error {
	dir := filepath.Dir(dbPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, os.ModePerm); err != nil {
			return err
		}
	}

	var err error
	DB, err = sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}

	// Test connection
	if err = DB.Ping(); err != nil {
		return err
	}

	log.Printf("Database connected successfully: %s", dbPath)

	// Create tables
	if err = createTables(); err != nil {
		return err
	}

	return nil
}

// createTables creates all required database tables
func createTables() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			username TEXT UNIQUE NOT NULL,
			email TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS manga (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			author TEXT,
			genres TEXT,
			status TEXT,
			total_chapters INTEGER,
			description TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS user_progress (
			user_id TEXT,
			manga_id TEXT,
			current_chapter INTEGER DEFAULT 0,
			status TEXT DEFAULT 'reading',
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			version INTEGER NOT NULL DEFAULT 1,
			PRIMARY KEY (user_id, manga_id),
			FOREIGN KEY (user_id) REFERENCES users(id),
			FOREIGN KEY (manga_id) REFERENCES manga(id)
		)`,
	}

	for _, query := range queries {
		if _, err := DB.Exec(query); err != nil {
			return err
		}
	}

	// Attempt migration if version column doesn't exist in existing user_progress table
	_, _ = DB.Exec(`ALTER TABLE user_progress ADD COLUMN version INTEGER NOT NULL DEFAULT 1`)

	log.Println("Database tables initialized successfully!")
	return nil
}

// SeedManga seeds the manga table if it is currently empty.
// It attempts to read data/manga_seed.json first.
func SeedManga() error {
	var count int
	if err := DB.QueryRow("SELECT COUNT(*) FROM manga").Scan(&count); err == nil && count > 0 {
		log.Printf("Manga table already has %d records. Skipping seeding.", count)
		return nil
	}

	var mangas []models.Manga

	// Try reading from data/manga_seed.json (checking common relative paths)
	seedPaths := []string{
		"data/manga_seed.json",
		"./data/manga_seed.json",
		"../data/manga_seed.json",
		"../../data/manga_seed.json",
	}

	var loadedFromJSON bool
	for _, p := range seedPaths {
		if data, err := os.ReadFile(p); err == nil {
			if err := json.Unmarshal(data, &mangas); err == nil && len(mangas) > 0 {
				log.Printf("Loaded %d manga items from seed file: %s", len(mangas), p)
				loadedFromJSON = true
				break
			}
		}
	}

	if !loadedFromJSON {
		log.Println("Seed JSON file not found or empty, using built-in seed list")
		mangas = []models.Manga{
			{ID: "one-piece", Title: "One Piece", Author: "Eiichiro Oda", Genres: []string{"Action", "Adventure", "Shounen"}, Status: "ongoing", TotalChapters: 1100, Description: "Pirate adventure"},
			{ID: "demon-slayer", Title: "Demon Slayer: Kimetsu no Yaiba", Author: "Koyoharu Gotouge", Genres: []string{"Action", "Supernatural", "Adventure", "Shounen"}, Status: "completed", TotalChapters: 205, Description: "Tanjiro fights demons after his family is slaughtered."},
			{ID: "attack-on-titan", Title: "Attack on Titan", Author: "Hajime Isayama", Genres: []string{"Action", "Drama", "Mystery", "Shounen"}, Status: "completed", TotalChapters: 139, Description: "Humanity fights for survival against giant Titans."},
			{ID: "naruto", Title: "Naruto", Author: "Masashi Kishimoto", Genres: []string{"Action", "Shounen"}, Status: "completed", TotalChapters: 700, Description: "Ninja story"},
			{ID: "dragon-ball", Title: "Dragon Ball", Author: "Akira Toriyama", Genres: []string{"Action", "Adventure", "Martial Arts", "Shounen"}, Status: "completed", TotalChapters: 520, Description: "Goku grows up training in martial arts and seeks the Dragon Balls."},
			{ID: "death-note", Title: "Death Note", Author: "Tsugumi Ohba", Genres: []string{"Mystery", "Psychological", "Supernatural", "Shounen"}, Status: "completed", TotalChapters: 108, Description: "A supernatural notebook that kills whoever's name is written."},
			{ID: "berserk", Title: "Berserk", Author: "Kentaro Miura", Genres: []string{"Dark Fantasy", "Action", "Seinen"}, Status: "ongoing", TotalChapters: 360, Description: "Guts battles demonic apostles in a grim fantasy world."},
			{ID: "nana", Title: "Nana", Author: "Ai Yazawa", Genres: []string{"Drama", "Music", "Romance", "Shoujo"}, Status: "hiatus", TotalChapters: 84, Description: "Two young women with the same name move to Tokyo."},
			{ID: "chihayafuru", Title: "Chihayafuru", Author: "Yuki Suetsugu", Genres: []string{"Sports", "Drama", "Josei"}, Status: "completed", TotalChapters: 247, Description: "A passionate girl strives to become competitive karuta champion."},
		}
	}

	tx, err := DB.Begin()
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

	if err := tx.Commit(); err != nil {
		return err
	}

	log.Printf("Successfully seeded %d manga entries", len(mangas))
	return nil
}

// Close closes the database connection
func Close() error {
	if DB != nil {
		return DB.Close()
	}
	return nil
}

