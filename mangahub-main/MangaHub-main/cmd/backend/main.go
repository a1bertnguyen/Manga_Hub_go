package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mangahub/internal/auth"
	"mangahub/internal/config"
	"mangahub/internal/database"
	"mangahub/internal/tcp"
	"mangahub/internal/udp"
	wsHub "mangahub/internal/websocket"
	"mangahub/pkg/models"
	pb "mangahub/proto"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	gorilla "github.com/gorilla/websocket"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

var upgrader = gorilla.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

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
	router := gin.Default()
	router.Use(cors.Default())

	// Health check
	router.GET("/api/health", healthHandler(cfg))

	// Public Auth Endpoints
	authGroup := router.Group("/api/auth")
	{
		authGroup.POST("/register", registerHandler)
		authGroup.POST("/login", loginHandler)
	}
	// Backwards-compatible aliases
	router.POST("/auth/register", registerHandler)
	router.POST("/auth/login", loginHandler)

	// Admin / Demo Release Endpoint (HTTP -> gRPC update -> UDP alert)
	router.POST("/api/admin/manga/:id/release", adminReleaseChapterHandler)
	router.POST("/admin/manga/:id/release", adminReleaseChapterHandler)

	// Protected Endpoints
	api := router.Group("/api")
	api.Use(auth.Middleware())
	{
		api.GET("/manga", getMangaHandler)
		api.GET("/manga/:id", getMangaDetailHandler)
		api.GET("/library", getLibraryHandler)
		api.POST("/library", addToLibraryHandler)
		api.PUT("/library/:mangaId/progress", updateProgressHandler)
	}

	// Backwards-compatible aliases
	protectedLegacy := router.Group("/")
	protectedLegacy.Use(auth.Middleware())
	{
		protectedLegacy.GET("/manga", getMangaHandler)
		protectedLegacy.GET("/manga/:id", getMangaDetailHandler)
		protectedLegacy.GET("/users/library", getLibraryHandler)
		protectedLegacy.POST("/users/library", addToLibraryHandler)
		protectedLegacy.PUT("/users/progress", updateProgressLegacyHandler)
	}

	// WebSocket discussion upgrade endpoint
	router.GET("/ws", handleWebSocketUpgrade)

	// Static web assets if present
	if _, err := os.Stat("web"); err == nil {
		router.Static("/web", "./web")
		router.GET("/", func(c *gin.Context) {
			c.File(filepath.Join("web", "search_manga.html"))
		})
	}

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

func healthHandler(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		catalogStatus := "unknown"
		if grpcCatalogClient != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()
			_, err := grpcCatalogClient.SearchManga(ctx, &pb.SearchMangaRequest{Query: ""})
			if err != nil {
				catalogStatus = "unreachable"
			} else {
				catalogStatus = "connected"
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"status":    "healthy",
			"timestamp": time.Now().Unix(),
			"services": gin.H{
				"http":         "running (:" + cfg.HTTPPort + ")",
				"tcp":          "running (:" + cfg.TCPPort + ")",
				"udp":          "running (:" + cfg.UDPPort + ")",
				"websocket":    "/ws",
				"grpc_catalog": catalogStatus,
			},
		})
	}
}

func startTCPListener(port string) {
	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Printf("[Backend TCP] Failed to listen on :%s: %v", port, err)
		return
	}
	defer listener.Close()

	log.Printf("[Backend TCP] Progress sync server listening on :%s", port)
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("[Backend TCP] Error accepting connection: %v", err)
			continue
		}
		go handleTCPConnection(conn)
	}
}

// handleTCPConnection manages a client session over TCP.
// Message Framing: One JSON object per newline. First message authenticates the session.
func handleTCPConnection(conn net.Conn) {
	defer conn.Close()

	remoteAddr := conn.RemoteAddr().String()
	log.Printf("[Backend TCP] Client connected: %s", remoteAddr)

	scanner := bufio.NewScanner(conn)
	// Read first message for session authentication
	if !scanner.Scan() {
		log.Printf("[Backend TCP] Disconnected before authentication: %s", remoteAddr)
		return
	}

	firstMsg := strings.TrimSpace(scanner.Text())
	var userID string

	// Attempt JSON decode of auth payload: {"type":"auth","token":"..."} or {"user_id":"..."}
	var authPayload struct {
		Type   string `json:"type"`
		Token  string `json:"token"`
		UserID string `json:"user_id"`
	}
	if err := json.Unmarshal([]byte(firstMsg), &authPayload); err == nil {
		if authPayload.Token != "" {
			if claims, err := auth.ValidateToken(authPayload.Token); err == nil {
				userID = claims.UserID
			}
		}
		if userID == "" && authPayload.UserID != "" {
			userID = authPayload.UserID
		}
	}

	// Fallback to plain text UserID if not JSON
	if userID == "" {
		userID = firstMsg
	}

	if userID == "" {
		conn.Write([]byte("{\"type\":\"error\",\"message\":\"authentication_failed\"}\n"))
		return
	}

	client := &tcp.Client{
		Conn:   conn,
		UserID: userID,
		Send:   make(chan []byte, 256),
	}

	// Send auth confirmation newline frame
	authConfirm, _ := json.Marshal(map[string]interface{}{
		"type":    "authenticated",
		"user_id": userID,
		"status":  "ok",
	})
	conn.Write(append(authConfirm, '\n'))

	log.Printf("[Backend TCP] Session authenticated: %s -> UserID: %s", remoteAddr, userID)

	tcp.GlobalHub.Register <- client
	go client.WritePump()
	client.ReadPump()
}

func handleWebSocketUpgrade(c *gin.Context) {
	username := c.Query("username")
	room := c.Query("room")
	if room == "" {
		room = "general"
	}
	if username == "" {
		username = "anonymous"
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[Backend WS] Upgrade error: %v", err)
		return
	}

	client := &wsHub.Client{
		Hub:      globalWSHub,
		Conn:     conn,
		Send:     make(chan []byte, 256),
		Username: username,
		Room:     room,
	}

	globalWSHub.Register <- client
	go client.WritePump()
	client.ReadPump()
}

func registerHandler(c *gin.Context) {
	var req models.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var exists int
	database.DB.QueryRow("SELECT COUNT(*) FROM users WHERE username = ?", req.Username).Scan(&exists)
	if exists > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "Username already exists"})
		return
	}

	passwordHash, err := auth.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
		return
	}

	userID := auth.GenerateID("usr")
	_, err = database.DB.Exec(`INSERT INTO users (id, username, email, password_hash) VALUES (?, ?, ?, ?)`,
		userID, req.Username, req.Email, passwordHash)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
		return
	}

	token, err := auth.GenerateToken(userID, req.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, models.LoginResponse{Token: token, Username: req.Username, UserID: userID})
}

func loginHandler(c *gin.Context) {
	var req models.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var user models.User
	err := database.DB.QueryRow("SELECT id, username, password_hash FROM users WHERE username = ?", req.Username).
		Scan(&user.ID, &user.Username, &user.PasswordHash)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	if !auth.CheckPassword(req.Password, user.PasswordHash) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}

	token, err := auth.GenerateToken(user.ID, user.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, models.LoginResponse{Token: token, Username: user.Username, UserID: user.ID})
}

// GET /api/manga?q=... - Obtains manga catalog through gRPC
func getMangaHandler(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		query = c.Query("search")
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
		if st.Code() == codes.Unavailable || st.Code() == codes.DeadlineExceeded {
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
		if st.Code() == codes.Unavailable || st.Code() == codes.DeadlineExceeded {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Catalog service unavailable: " + st.Message()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Catalog query failed: " + st.Message()})
		return
	}

	c.JSON(http.StatusOK, resp.Manga)
}

// POST /api/admin/manga/:id/release - Admin/demo release -> gRPC update -> UDP alert
type AdminReleaseReq struct {
	Chapter int    `json:"chapter" binding:"required,gt=0"`
	Title   string `json:"title"`
}

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
		if st.Code() == codes.Unavailable || st.Code() == codes.DeadlineExceeded {
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

type AddToLibraryReq struct {
	MangaID string `json:"manga_id" binding:"required"`
	Status  string `json:"status"`
}

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

func updateProgressHandler(c *gin.Context) {
	userID := c.GetString("user_id")
	mangaID := c.Param("mangaId")
	username := c.GetString("username")

	var req UpdateProgressReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate max chapters via gRPC catalog
	mangaTitle := mangaID
	if grpcCatalogClient != nil {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if mResp, err := grpcCatalogClient.GetManga(ctx, &pb.GetMangaRequest{Id: mangaID}); err == nil && mResp.Manga != nil {
			mangaTitle = mResp.Manga.Title
			if mResp.Manga.TotalChapters > 0 && int32(req.CurrentChapter) > mResp.Manga.TotalChapters {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": fmt.Sprintf("Chapter %d exceeds manga total chapters (%d)", req.CurrentChapter, mResp.Manga.TotalChapters),
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
	`, req.CurrentChapter, req.Status, userID, mangaID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update progress"})
		return
	}

	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Manga not in user library"})
		return
	}

	// Fetch updated version
	var newVersion int
	database.DB.QueryRow("SELECT version FROM user_progress WHERE user_id = ? AND manga_id = ?", userID, mangaID).Scan(&newVersion)

	// Broadcast via TCP ONLY to same user's other sessions
	tcp.GlobalHub.BroadcastProgress(models.UserProgress{
		UserID:         userID,
		MangaID:        mangaID,
		CurrentChapter: req.CurrentChapter,
		Status:         req.Status,
		Version:        newVersion,
	}, username, mangaTitle)

	c.JSON(http.StatusOK, gin.H{
		"message":  "Progress updated and TCP event dispatched",
		"version":  newVersion,
		"chapter":  req.CurrentChapter,
		"manga_id": mangaID,
	})
}

// Legacy handler for PUT /users/progress
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

	c.Params = append(c.Params, gin.Param{Key: "mangaId", Value: req.MangaID})
	updateProgressHandler(c)
}
