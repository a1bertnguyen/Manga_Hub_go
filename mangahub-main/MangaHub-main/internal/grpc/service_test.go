package grpc

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"mangahub/internal/database"
	pb "mangahub/proto"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func setupTestDB(t *testing.T) *sql.DB {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "grpc_test.db")

	if err := database.Initialize(dbPath); err != nil {
		t.Fatalf("Failed to initialize test DB: %v", err)
	}

	t.Cleanup(func() {
		_ = database.Close()
	})

	// Insert test manga
	_, err := database.DB.Exec(`
		INSERT INTO manga (id, title, author, genres, status, total_chapters, description)
		VALUES 
		('one-piece', 'One Piece', 'Eiichiro Oda', 'Action,Adventure,Shounen', 'ongoing', 1100, 'Pirates'),
		('naruto', 'Naruto', 'Masashi Kishimoto', 'Action,Shounen', 'completed', 700, 'Ninjas'),
		('berserk', 'Berserk', 'Kentaro Miura', 'Dark Fantasy,Action,Seinen', 'ongoing', 360, 'Guts')
	`)
	if err != nil {
		t.Fatalf("Failed to seed manga for grpc test: %v", err)
	}

	return database.DB
}

func TestMangaServiceServer_GetManga(t *testing.T) {
	db := setupTestDB(t)
	server := NewMangaServiceServer(db)
	ctx := context.Background()

	// 1. Success case
	resp, err := server.GetManga(ctx, &pb.GetMangaRequest{Id: "one-piece"})
	if err != nil {
		t.Fatalf("GetManga failed: %v", err)
	}
	if resp.Manga == nil || resp.Manga.Title != "One Piece" {
		t.Fatalf("Expected title 'One Piece', got %+v", resp.Manga)
	}
	if len(resp.Manga.Genres) != 3 {
		t.Errorf("Expected 3 genres, got %d", len(resp.Manga.Genres))
	}

	// 2. Not found case
	_, err = server.GetManga(ctx, &pb.GetMangaRequest{Id: "non-existent-manga"})
	if err == nil {
		t.Fatal("Expected error for non-existent manga, got nil")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.NotFound {
		t.Errorf("Expected codes.NotFound, got %v (st=%+v)", err, st)
	}
}

func TestMangaServiceServer_SearchManga(t *testing.T) {
	db := setupTestDB(t)
	server := NewMangaServiceServer(db)
	ctx := context.Background()

	// 1. Query by title substring
	resp, err := server.SearchManga(ctx, &pb.SearchMangaRequest{Query: "piece"})
	if err != nil {
		t.Fatalf("SearchManga failed: %v", err)
	}
	if len(resp.Mangas) != 1 || resp.Mangas[0].Id != "one-piece" {
		t.Errorf("Expected 1 result for 'piece', got %d", len(resp.Mangas))
	}

	// 2. Query by genre
	resp, err = server.SearchManga(ctx, &pb.SearchMangaRequest{Genre: "Seinen"})
	if err != nil {
		t.Fatalf("SearchManga by genre failed: %v", err)
	}
	if len(resp.Mangas) != 1 || resp.Mangas[0].Id != "berserk" {
		t.Errorf("Expected berserk for Seinen, got %d results", len(resp.Mangas))
	}

	// 3. Query no matches
	resp, err = server.SearchManga(ctx, &pb.SearchMangaRequest{Query: "non-existent-xyz"})
	if err != nil {
		t.Fatalf("SearchManga failed: %v", err)
	}
	if len(resp.Mangas) != 0 {
		t.Errorf("Expected 0 results, got %d", len(resp.Mangas))
	}
}

func TestMangaServiceServer_UpdateChapters(t *testing.T) {
	db := setupTestDB(t)
	server := NewMangaServiceServer(db)
	ctx := context.Background()

	// 1. Valid update
	resp, err := server.UpdateChapters(ctx, &pb.UpdateChaptersRequest{
		Id:            "one-piece",
		TotalChapters: 1105,
	})
	if err != nil {
		t.Fatalf("UpdateChapters failed: %v", err)
	}
	if !resp.Success || resp.TotalChapters != 1105 {
		t.Errorf("Unexpected response: %+v", resp)
	}

	// Verify persistence in DB
	var total int
	_ = db.QueryRow("SELECT total_chapters FROM manga WHERE id = 'one-piece'").Scan(&total)
	if total != 1105 {
		t.Errorf("Expected DB total_chapters=1105, got %d", total)
	}

	// 2. Negative chapters -> InvalidArgument
	_, err = server.UpdateChapters(ctx, &pb.UpdateChaptersRequest{
		Id:            "one-piece",
		TotalChapters: -5,
	})
	if err == nil {
		t.Fatal("Expected error for negative chapters, got nil")
	}
	if st, _ := status.FromError(err); st.Code() != codes.InvalidArgument {
		t.Errorf("Expected InvalidArgument, got %v", err)
	}

	// 3. Non-existent manga -> NotFound
	_, err = server.UpdateChapters(ctx, &pb.UpdateChaptersRequest{
		Id:            "ghost-manga",
		TotalChapters: 20,
	})
	if err == nil {
		t.Fatal("Expected error for non-existent manga, got nil")
	}
	if st, _ := status.FromError(err); st.Code() != codes.NotFound {
		t.Errorf("Expected NotFound, got %v", err)
	}
}

func TestMangaServiceServer_UpdateProgress(t *testing.T) {
	db := setupTestDB(t)
	server := NewMangaServiceServer(db)
	ctx := context.Background()

	// 1. Valid progress update
	resp, err := server.UpdateProgress(ctx, &pb.UpdateProgressRequest{
		UserId:         "usr_tester",
		MangaId:        "one-piece",
		CurrentChapter: 45,
	})
	if err != nil {
		t.Fatalf("UpdateProgress failed: %v", err)
	}
	if !resp.Success {
		t.Errorf("Expected Success=true, got %+v", resp)
	}

	// 2. Negative chapter -> InvalidArgument
	_, err = server.UpdateProgress(ctx, &pb.UpdateProgressRequest{
		UserId:         "usr_tester",
		MangaId:        "one-piece",
		CurrentChapter: -1,
	})
	if err == nil {
		t.Fatal("Expected error for negative current_chapter, got nil")
	}
	if st, _ := status.FromError(err); st.Code() != codes.InvalidArgument {
		t.Errorf("Expected InvalidArgument, got %v", err)
	}

	// 3. Non-existent manga -> NotFound
	_, err = server.UpdateProgress(ctx, &pb.UpdateProgressRequest{
		UserId:         "usr_tester",
		MangaId:        "ghost-manga",
		CurrentChapter: 10,
	})
	if err == nil {
		t.Fatal("Expected error for ghost manga, got nil")
	}
	if st, _ := status.FromError(err); st.Code() != codes.NotFound {
		t.Errorf("Expected NotFound, got %v", err)
	}
}
