package main

import (
	"context"
	"net/http"
	"strings"
	"time"

	"mangahub/internal/udp"
	pb "mangahub/proto"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// isCatalogUnavailable detects whether a gRPC error signifies that the catalog microservice is down or unreachable
func isCatalogUnavailable(err error) bool {
	if err == nil {
		return false
	}
	st, _ := status.FromError(err)
	if st.Code() == codes.Unavailable || st.Code() == codes.DeadlineExceeded || st.Code() == codes.Canceled {
		return true
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "connection is closing") ||
		strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "transientfailure") ||
		strings.Contains(errStr, "deadline") ||
		strings.Contains(errStr, "unavailable")
}

// GET /api/manga?q=... - Obtains manga catalog through gRPC
func getMangaHandler(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		query = c.Query("search")
	}
	if query == "" {
		query = c.Query("title")
	}
	genre := c.Query("genre")

	if grpcCatalogClient == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Catalog service is unavailable"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	resp, err := grpcCatalogClient.SearchManga(ctx, &pb.SearchMangaRequest{
		Query: query,
		Genre: genre,
	})
	if err != nil {
		st, _ := status.FromError(err)
		if isCatalogUnavailable(err) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Catalog service unavailable: " + st.Message()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Catalog query failed: " + st.Message()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"manga": resp.Mangas,
		"count": resp.Count,
	})
}

// GET /api/manga/:id - Obtains manga detail through gRPC
func getMangaDetailHandler(c *gin.Context) {
	id := c.Param("id")

	if grpcCatalogClient == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Catalog service is unavailable"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	resp, err := grpcCatalogClient.GetManga(ctx, &pb.GetMangaRequest{Id: id})
	if err != nil {
		st, _ := status.FromError(err)
		if st.Code() == codes.NotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Manga not found"})
			return
		}
		if isCatalogUnavailable(err) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Catalog service unavailable: " + st.Message()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Catalog query failed: " + st.Message()})
		return
	}

	c.JSON(http.StatusOK, resp.Manga)
}

// AdminReleaseReq defines the request body for releasing a new chapter
type AdminReleaseReq struct {
	Chapter int    `json:"chapter" binding:"required,gt=0"`
	Title   string `json:"title"`
}

// POST /api/admin/manga/:id/release - Admin/demo release -> gRPC update -> UDP alert
func adminReleaseChapterHandler(c *gin.Context) {
	id := c.Param("id")
	var req AdminReleaseReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input: chapter (integer > 0) is required"})
		return
	}

	if grpcCatalogClient == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Catalog service is unavailable"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()

	// 1. Update total chapters on gRPC catalog service
	resp, err := grpcCatalogClient.UpdateChapters(ctx, &pb.UpdateChaptersRequest{
		Id:            id,
		TotalChapters: int32(req.Chapter),
	})
	if err != nil {
		st, _ := status.FromError(err)
		if st.Code() == codes.NotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Manga not found in catalog"})
			return
		}
		if isCatalogUnavailable(err) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Catalog service unavailable: " + st.Message()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update catalog: " + st.Message()})
		return
	}

	// 2. Fetch manga title for informative notification
	title := req.Title
	if title == "" {
		if mResp, err := grpcCatalogClient.GetManga(ctx, &pb.GetMangaRequest{Id: id}); err == nil && mResp.Manga != nil {
			title = mResp.Manga.Title
		} else {
			title = id
		}
	}

	// 3. Broadcast UDP event to all subscribed TUI clients
	udp.GlobalHub.BroadcastChapterRelease(id, title, req.Chapter)

	c.JSON(http.StatusOK, gin.H{
		"message":        "New chapter released and UDP alert broadcasted",
		"manga_id":       id,
		"title":          title,
		"total_chapters": resp.TotalChapters,
	})
}

