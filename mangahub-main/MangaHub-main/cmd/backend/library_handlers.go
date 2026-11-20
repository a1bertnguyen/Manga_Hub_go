package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"mangahub/internal/database"
	"mangahub/internal/tcp"
	"mangahub/pkg/models"
	pb "mangahub/proto"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type AddToLibraryReq struct {
	MangaID string `json:"manga_id" binding:"required"`
	Status  string `json:"status"`
}

// POST /api/library - Adds a manga to current user's reading library
func addToLibraryHandler(c *gin.Context) {
	userID := c.GetString("user_id")
	var req AddToLibraryReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if _, ok := requireCatalogManga(c, req.MangaID); !ok {
		return
	}
	if !validReadingStatus(req.Status) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid reading status"})
		return
	}

	status := req.Status
	if status == "" {
		status = "reading"
	}

	_, err := database.DB.Exec(`
		INSERT INTO user_progress (user_id, manga_id, current_chapter, status, version)
		VALUES (?, ?, 0, ?, 1)
		ON CONFLICT(user_id, manga_id) DO UPDATE SET status = excluded.status
	`, userID, req.MangaID, status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add to library"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Added to library"})
}

// GET /api/library - Retrieves reading library for authenticated user
func getLibraryHandler(c *gin.Context) {
	userID := c.GetString("user_id")

	rows, err := database.DB.Query(`
		SELECT up.manga_id, up.current_chapter, up.status, up.version
		FROM user_progress up
		WHERE up.user_id = ?
	`, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch library"})
		return
	}
	defer rows.Close()

	type Item struct {
		MangaID        string `json:"manga_id"`
		Title          string `json:"title"`
		CurrentChapter int    `json:"current_chapter"`
		Status         string `json:"status"`
		Version        int    `json:"version"`
	}

	items := make([]Item, 0)
	for rows.Next() {
		var i Item
		if err := rows.Scan(&i.MangaID, &i.CurrentChapter, &i.Status, &i.Version); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read library"})
			return
		}
		i.Title = i.MangaID
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read library"})
		return
	}
	// Release the SQLite connection before enriching metadata through another service.
	_ = rows.Close()

	// Enrich titles via gRPC catalog if available
	if grpcCatalogClient != nil {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		for idx := range items {
			if ctx.Err() != nil {
				break
			}
			if mResp, err := grpcCatalogClient.GetManga(ctx, &pb.GetMangaRequest{Id: items[idx].MangaID}); err == nil && mResp.Manga != nil {
				items[idx].Title = mResp.Manga.Title
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"library": items, "count": len(items)})
}

type UpdateProgressReq struct {
	CurrentChapter int    `json:"current_chapter" binding:"gte=0"`
	Status         string `json:"status"`
}

// PUT /api/library/:mangaId/progress - Updates reading chapter & triggers TCP sync with per-user isolation
func updateProgressHandler(c *gin.Context) {
	mangaID := c.Param("mangaId")
	var req UpdateProgressReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	executeProgressUpdate(c, mangaID, req.CurrentChapter, req.Status)
}

// updateProgressLegacyHandler handles backwards-compatible PUT /users/progress
func updateProgressLegacyHandler(c *gin.Context) {
	var req struct {
		MangaID        string `json:"manga_id" binding:"required"`
		CurrentChapter int    `json:"current_chapter" binding:"gte=0"`
		Status         string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	executeProgressUpdate(c, req.MangaID, req.CurrentChapter, req.Status)
}

// executeProgressUpdate carries out progress update, validation against max chapters, and TCP broadcast
func executeProgressUpdate(c *gin.Context, mangaID string, currentChapter int, status string) {
	userID := c.GetString("user_id")
	username := c.GetString("username")

	if currentChapter < 0 || !validReadingStatus(status) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid chapter or reading status"})
		return
	}
	manga, ok := requireCatalogManga(c, mangaID)
	if !ok {
		return
	}
	// Compare without narrowing to int32: very large input must not wrap around.
	if int64(currentChapter) > int64(manga.TotalChapters) {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Chapter %d exceeds manga total chapters (%d)", currentChapter, manga.TotalChapters)})
		return
	}
	mangaTitle := manga.Title
	var newVersion int
	// RETURNING binds the returned version to this exact committed update.
	err := database.DB.QueryRowContext(c.Request.Context(), `
  UPDATE user_progress SET current_chapter = ?,
  status = COALESCE(NULLIF(?, ''), status), updated_at = CURRENT_TIMESTAMP,
  version = version + 1 WHERE user_id = ? AND manga_id = ?
  RETURNING version, status
 `, currentChapter, status, userID, mangaID).Scan(&newVersion, &status)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Manga not in user library. Please add it first."})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update progress"})
		return
	}

	// Broadcast via TCP ONLY to same user's other sessions (Strict User Isolation)
	tcp.GlobalHub.BroadcastProgress(models.UserProgress{
		UserID:         userID,
		MangaID:        mangaID,
		CurrentChapter: currentChapter,
		Status:         status,
		Version:        newVersion,
	}, username, mangaTitle)

	c.JSON(http.StatusOK, gin.H{
		"message":  "Progress updated and TCP event dispatched",
		"version":  newVersion,
		"chapter":  currentChapter,
		"manga_id": mangaID,
	})
}

// requireCatalogManga is the shared boundary for all writes needing catalog validation.
func requireCatalogManga(c *gin.Context, id string) (*pb.Manga, bool) {
	if grpcCatalogClient == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Catalog service unavailable"})
		return nil, false
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	response, err := grpcCatalogClient.GetManga(ctx, &pb.GetMangaRequest{Id: id})
	if err != nil {
		code := http.StatusInternalServerError
		switch status.Code(err) {
		case codes.NotFound:
			code = http.StatusNotFound
		case codes.InvalidArgument:
			code = http.StatusBadRequest
		case codes.Unavailable, codes.DeadlineExceeded, codes.Canceled:
			code = http.StatusServiceUnavailable
		}
		c.JSON(code, gin.H{"error": "Catalog validation failed: " + status.Convert(err).Message()})
		return nil, false
	}
	if response == nil || response.Manga == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Catalog returned no manga"})
		return nil, false
	}
	return response.Manga, true
}

func validReadingStatus(value string) bool {
	switch value {
	case "", "reading", "completed", "plan_to_read", "on-hold", "dropped":
		return true
	default:
		return false
	}
}
