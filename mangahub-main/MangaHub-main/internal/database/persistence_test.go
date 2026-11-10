package database

import (
	"path/filepath"
	"testing"
)

func TestDatabaseInitializeAndSeed(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_catalog.db")

	if err := Initialize(dbPath); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	defer Close()

	if err := SeedManga(); err != nil {
		t.Fatalf("SeedManga failed: %v", err)
	}

	var count int
	if err := DB.QueryRow("SELECT COUNT(*) FROM manga").Scan(&count); err != nil {
		t.Fatalf("Failed to query manga count: %v", err)
	}
	if count == 0 {
		t.Errorf("Expected manga count > 0 after seeding, got 0")
	}

	// Verify idempotency of SeedManga
	if err := SeedManga(); err != nil {
		t.Fatalf("Second SeedManga call should succeed without error: %v", err)
	}
}

func TestDataPersistenceAcrossRestart(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "persist_test.db")

	// Phase 1: Initialize, insert user and user_progress
	if err := Initialize(dbPath); err != nil {
		t.Fatalf("Phase 1 Initialize failed: %v", err)
	}

	_, err := DB.Exec(`INSERT INTO users (id, username, email, password_hash) VALUES (?, ?, ?, ?)`,
		"usr_test_1", "john_doe", "john@example.com", "hash123")
	if err != nil {
		t.Fatalf("Insert user failed: %v", err)
	}

	_, err = DB.Exec(`INSERT INTO manga (id, title, author, genres, status, total_chapters, description)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"manga_test_1", "Test Manga", "Author X", "Action", "ongoing", 50, "Desc")
	if err != nil {
		t.Fatalf("Insert manga failed: %v", err)
	}

	_, err = DB.Exec(`INSERT INTO user_progress (user_id, manga_id, current_chapter, status, version)
		VALUES (?, ?, ?, ?, ?)`,
		"usr_test_1", "manga_test_1", 12, "reading", 1)
	if err != nil {
		t.Fatalf("Insert user_progress failed: %v", err)
	}

	// Close database to simulate service shutdown
	if err := Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Phase 2: Re-open database (simulate restart)
	if err := Initialize(dbPath); err != nil {
		t.Fatalf("Phase 2 Initialize after restart failed: %v", err)
	}
	defer Close()

	// Verify user persisted
	var username string
	if err := DB.QueryRow("SELECT username FROM users WHERE id = ?", "usr_test_1").Scan(&username); err != nil {
		t.Fatalf("User not found after restart: %v", err)
	}
	if username != "john_doe" {
		t.Errorf("Expected username 'john_doe', got %s", username)
	}

	// Verify user progress and version persisted
	var chapter int
	var status string
	var version int
	if err := DB.QueryRow("SELECT current_chapter, status, version FROM user_progress WHERE user_id = ? AND manga_id = ?",
		"usr_test_1", "manga_test_1").Scan(&chapter, &status, &version); err != nil {
		t.Fatalf("Progress not found after restart: %v", err)
	}

	if chapter != 12 || status != "reading" || version != 1 {
		t.Errorf("Progress mismatch after restart: chapter=%d (expected 12), status=%s (expected reading), version=%d (expected 1)",
			chapter, status, version)
	}
}

func TestOptimisticConcurrencyLocking(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "concurrency_test.db")

	if err := Initialize(dbPath); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	defer Close()

	_, _ = DB.Exec(`INSERT INTO users (id, username, email, password_hash) VALUES ('usr_c', 'user_c', 'c@c.com', 'h')`)
	_, _ = DB.Exec(`INSERT INTO manga (id, title, total_chapters) VALUES ('manga_c', 'Title C', 100)`)
	_, _ = DB.Exec(`INSERT INTO user_progress (user_id, manga_id, current_chapter, status, version) VALUES ('usr_c', 'manga_c', 5, 'reading', 1)`)

	// Client 1 reads version 1 and updates to chapter 6 (version becomes 2)
	res, err := DB.Exec(`
		UPDATE user_progress 
		SET current_chapter = ?, version = version + 1 
		WHERE user_id = ? AND manga_id = ? AND version = ?`,
		6, "usr_c", "manga_c", 1)
	if err != nil {
		t.Fatalf("Client 1 update failed: %v", err)
	}
	rowsAffected, _ := res.RowsAffected()
	if rowsAffected != 1 {
		t.Fatalf("Expected 1 row affected for valid version, got %d", rowsAffected)
	}

	// Client 2 attempts update with stale version 1 (which was already incremented to 2)
	staleRes, err := DB.Exec(`
		UPDATE user_progress 
		SET current_chapter = ?, version = version + 1 
		WHERE user_id = ? AND manga_id = ? AND version = ?`,
		7, "usr_c", "manga_c", 1)
	if err != nil {
		t.Fatalf("Client 2 update execution failed: %v", err)
	}
	staleRowsAffected, _ := staleRes.RowsAffected()
	if staleRowsAffected != 0 {
		t.Errorf("Optimistic concurrency failure: Stale update succeeded! Rows affected: %d", staleRowsAffected)
	}
}

