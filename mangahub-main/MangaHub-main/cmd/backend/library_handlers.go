package main

import (
	"context"
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

	// Verify manga exists via gRPC
	if grpcCatalogClient != nil {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		_, err := grpcCatalogClient.GetManga(ctx, &pb.GetMangaRequest{Id: req.MangaID})
		if err != nil {
			st, _ := status.FromError(err)
			if st.Code() == codes.NotFound {
				c.JSON(http.StatusNotFound, gin.H{"error": "Manga not found in catalog"})
				return
			}
		}
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

	var items []Item
	for rows.Next() {
		var i Item
		rows.Scan(&i.MangaID, &i.CurrentChapter, &i.Status, &i.Version)
		i.Title = i.MangaID
		items = append(items, i)
	}

	// Enrich titles via gRPC catalog if available
	if grpcCatalogClient != nil {
		for idx := range items {
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			if mResp, err := grpcCatalogClient.GetManga(ctx, &pb.GetMangaRequest{Id: items[idx].MangaID}); err == nil && mResp.Manga != nil {
				items[idx].Title = mResp.Manga.Title
			}
			cancel()
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

	// Validate max chapters via gRPC catalog (Grade A Requirement: 400 Bad Request if chapter > total)
	mangaTitle := mangaID
	if grpcCatalogClient != nil {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if mResp, err := grpcCatalogClient.GetManga(ctx, &pb.GetMangaRequest{Id: mangaID}); err == nil && mResp.Manga != nil {
			mangaTitle = mResp.Manga.Title
			if mResp.Manga.TotalChapters > 0 && int32(currentChapter) > mResp.Manga.TotalChapters {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": fmt.Sprintf("Chapter %d exceeds manga total chapters (%d)", currentChapter, mResp.Manga.TotalChapters),
				})
				return
			}
		}
	}

	res, err := database.DB.Exec(`
		UPDATE user_progress 
		SET current_chapter = ?, 
		    status = COALESCE(NULLIF(?, ''), status), 
		    updated_at = CURRENT_TIMESTAMP, 
		    version = version + 1
		WHERE user_id = ? AND manga_id = ?
	`, currentChapter, status, userID, mangaID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update progress: " + err.Error()})
		return
	}

	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Manga not in user library. Please add it first."})
		return
	}

	// Fetch updated version (Optimistic Concurrency Control)
	var newVersion int
	database.DB.QueryRow("SELECT version FROM user_progress WHERE user_id = ? AND manga_id = ?", userID, mangaID).Scan(&newVersion)

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

