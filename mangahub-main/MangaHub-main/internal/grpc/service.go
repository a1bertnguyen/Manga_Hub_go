package grpc

import (
	"context"
	"database/sql"
	"log"
	"strings"

	pb "mangahub/proto"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// MangaServiceServer implements the gRPC service
type MangaServiceServer struct {
	pb.UnimplementedMangaServiceServer
	db *sql.DB
}

// NewMangaServiceServer creates a new gRPC service implementation for manga operations
// This server provides gRPC endpoints for getting, searching, and updating manga progress.
func NewMangaServiceServer(db *sql.DB) *MangaServiceServer {
	return &MangaServiceServer{db: db}
}

// GetManga retrieves a manga by ID
func (s *MangaServiceServer) GetManga(ctx context.Context, req *pb.GetMangaRequest) (*pb.GetMangaResponse, error) {
	log.Printf("gRPC GetManga called: id=%s", req.Id)

	var manga pb.Manga
	var genresStr string

	err := s.db.QueryRowContext(ctx,
		"SELECT id, title, author, genres, status, total_chapters, description FROM manga WHERE id = ?",
		req.Id,
	).Scan(&manga.Id, &manga.Title, &manga.Author, &genresStr, &manga.Status, &manga.TotalChapters, &manga.Description)

	if err == sql.ErrNoRows {
		return nil, status.Errorf(codes.NotFound, "manga not found: id=%s", req.Id)
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "database error: %v", err)
	}

	// Convert genres string to array
	if genresStr != "" {
		manga.Genres = strings.Split(genresStr, ",")
	}

	return &pb.GetMangaResponse{Manga: &manga}, nil
}

// SearchManga searches for manga
func (s *MangaServiceServer) SearchManga(ctx context.Context, req *pb.SearchMangaRequest) (*pb.SearchMangaResponse, error) {
	log.Printf("gRPC SearchManga called: query=%s, genre=%s", req.Query, req.Genre)

	query := "SELECT id, title, author, genres, status, total_chapters, description FROM manga WHERE 1=1"
	args := []interface{}{}

	if req.Query != "" {
		query += " AND (title LIKE ? OR author LIKE ?)"
		pattern := "%" + req.Query + "%"
		args = append(args, pattern, pattern)
	}

	if req.Genre != "" {
		query += " AND genres LIKE ?"
		args = append(args, "%"+req.Genre+"%")
	}

	query += " LIMIT 100"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "search failed: %v", err)
	}
	defer rows.Close()

	var mangas []*pb.Manga
	for rows.Next() {
		var manga pb.Manga
		var genresStr string

		err := rows.Scan(&manga.Id, &manga.Title, &manga.Author, &genresStr, &manga.Status, &manga.TotalChapters, &manga.Description)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "cannot read manga search result: %v", err)
		}

		if genresStr != "" {
			manga.Genres = strings.Split(genresStr, ",")
		}

		mangas = append(mangas, &manga)
	}
	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "cannot finish manga search: %v", err)
	}

	return &pb.SearchMangaResponse{
		Mangas: mangas,
		Count:  int32(len(mangas)),
	}, nil
}

// UpdateProgress updates reading progress
func (s *MangaServiceServer) UpdateProgress(ctx context.Context, req *pb.UpdateProgressRequest) (*pb.UpdateProgressResponse, error) {
	log.Printf("gRPC UpdateProgress called: user=%s, manga=%s, chapter=%d", req.UserId, req.MangaId, req.CurrentChapter)

	// Validate input
	if req.UserId == "" || req.MangaId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id and manga_id are required")
	}

	if req.CurrentChapter < 0 {
		return nil, status.Error(codes.InvalidArgument, "current_chapter must be non-negative")
	}

	// Check if manga exists
	var exists bool
	err := s.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM manga WHERE id = ?)", req.MangaId).Scan(&exists)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "database error: %v", err)
	}
	if !exists {
		return nil, status.Errorf(codes.NotFound, "manga not found: id=%s", req.MangaId)
	}

	// Update or insert progress
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO user_progress (user_id, manga_id, current_chapter, updated_at)
		 VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(user_id, manga_id) 
		 DO UPDATE SET current_chapter = ?, updated_at = CURRENT_TIMESTAMP`,
		req.UserId, req.MangaId, req.CurrentChapter, req.CurrentChapter,
	)

	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update progress: %v", err)
	}

	return &pb.UpdateProgressResponse{
		Success: true,
		Message: "Progress updated successfully",
	}, nil
}

// UpdateChapters updates the total chapters of a manga series (e.g. when a new chapter is released)
func (s *MangaServiceServer) UpdateChapters(ctx context.Context, req *pb.UpdateChaptersRequest) (*pb.UpdateChaptersResponse, error) {
	log.Printf("gRPC UpdateChapters called: id=%s, total_chapters=%d", req.Id, req.TotalChapters)

	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	if req.TotalChapters < 0 {
		return nil, status.Error(codes.InvalidArgument, "total_chapters must be non-negative")
	}

	// Compare and update in one statement so simultaneous releases cannot lower
	// the count or both succeed for the same chapter.
	var total int32
	err := s.db.QueryRowContext(ctx, `
		UPDATE manga SET total_chapters = ?
		WHERE id = ? AND total_chapters < ?
		RETURNING total_chapters
	`, req.TotalChapters, req.Id, req.TotalChapters).Scan(&total)
	if err == sql.ErrNoRows {
		err = s.db.QueryRowContext(ctx, "SELECT total_chapters FROM manga WHERE id = ?", req.Id).Scan(&total)
		if err == sql.ErrNoRows {
			return nil, status.Errorf(codes.NotFound, "manga not found: id=%s", req.Id)
		}
		if err == nil {
			return nil, status.Errorf(codes.FailedPrecondition, "new chapter must exceed the current total (%d)", total)
		}
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update chapters: %v", err)
	}

	return &pb.UpdateChaptersResponse{
		Success:       true,
		Message:       "Chapters updated successfully",
		TotalChapters: total,
	}, nil
}
