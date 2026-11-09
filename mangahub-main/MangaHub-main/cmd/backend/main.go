package main

import (
	"database/sql"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"mangahub/internal/auth"
	"mangahub/internal/config"
	"mangahub/internal/database"
	"mangahub/internal/tcp"
	"mangahub/internal/udp"
	wsHub "mangahub/internal/websocket"
	"mangahub/pkg/models"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	gorilla "github.com/gorilla/websocket"
)

var upgrader = gorilla.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

var globalWSHub *wsHub.Hub

func main() {
	cfg := config.Load()

	// 1. Initialize SQLite Database for Users & User Library
	if err := database.Initialize(cfg.BackendDBPath); err != nil {
		log.Fatalf("[Backend] Failed to initialize database: %v", err)
	}
	defer database.Close()

	// Seed fallback manga data if local table is empty (for transition to gRPC catalog)
	if err := database.SeedManga(); err != nil {
		log.Printf("[Backend] Warning seeding manga: %v", err)
	}

	// 2. Start TCP Progress Sync Hub & Listener
	go tcp.GlobalHub.Run()
	go startTCPListener(cfg.TCPPort)

	// 3. Start UDP Notification Hub & Listener
	go udp.GlobalHub.Run()
	go udp.StartUDPListener(":" + cfg.UDPPort)

	// 4. Start WebSocket Discussion Hub
	globalWSHub = wsHub.NewHub()
	go globalWSHub.Run()

	// 5. Initialize Gin Engine (HTTP REST + WebSocket on HTTPPort)
	router := gin.Default()
	router.Use(cors.Default())

	// Health check
	router.GET("/api/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "healthy",
			"timestamp": time.Now().Unix(),
			"services": gin.H{
				"http":      "running",
				"tcp":       ":" + cfg.TCPPort,
				"udp":       ":" + cfg.UDPPort,
				"websocket": "/ws",
			},
		})
	})

	// Public Auth Endpoints
	authGroup := router.Group("/api/auth")
	{
		authGroup.POST("/register", registerHandler)
		authGroup.POST("/login", loginHandler)
	}
	// Backwards-compatible aliases without /api
	router.POST("/auth/register", registerHandler)
	router.POST("/auth/login", loginHandler)

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
	log.Printf("   Database       : %s", cfg.BackendDBPath)
	log.Printf("==================================================")

	if err := router.Run(":" + cfg.HTTPPort); err != nil {
		log.Fatalf("[Backend] Gin HTTP Server error: %v", err)
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

func handleTCPConnection(conn net.Conn) {
	defer conn.Close()

	remoteAddr := conn.RemoteAddr().String()
	log.Printf("[Backend TCP] Client connected: %s", remoteAddr)

	fmt.Fprintf(conn, "Welcome to MangaHub Progress Sync!\nEnter UserID:\n")

	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil || n == 0 {
		return
	}

	userID := string(buf[:n])
	// Trim newline / whitespace
	for len(userID) > 0 && (userID[len(userID)-1] == '\r' || userID[len(userID)-1] == '\n' || userID[len(userID)-1] == ' ') {
		userID = userID[:len(userID)-1]
	}

	client := &tcp.Client{
		Conn:   conn,
		UserID: userID,
		Send:   make(chan []byte, 256),
	}

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

func getMangaDetailHandler(c *gin.Context) {
	id := c.Param("id")
	var m models.Manga
	err := database.DB.QueryRow(`SELECT id, title, author, genres, status, total_chapters, description FROM manga WHERE id = ?`, id).
		Scan(&m.ID, &m.Title, &m.Author, &m.GenresString, &m.Status, &m.TotalChapters, &m.Description)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Manga not found"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	m.PostScan()
	c.JSON(http.StatusOK, m)
}

func getMangaHandler(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		query = c.Query("search")
	}

	var rows *sql.Rows
	var err error

	if query != "" {
		rows, err = database.DB.Query(`
			SELECT id, title, author, genres, status, total_chapters, description 
			FROM manga WHERE LOWER(title) LIKE LOWER(?) ORDER BY title
		`, "%"+query+"%")
	} else {
		rows, err = database.DB.Query(`
			SELECT id, title, author, genres, status, total_chapters, description 
			FROM manga ORDER BY title
		`)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch manga"})
		return
	}
	defer rows.Close()

	var list []models.Manga
	for rows.Next() {
		var m models.Manga
		if err := rows.Scan(&m.ID, &m.Title, &m.Author, &m.GenresString, &m.Status, &m.TotalChapters, &m.Description); err == nil {
			m.PostScan()
			list = append(list, m)
		}
	}

	c.JSON(http.StatusOK, gin.H{"manga": list, "count": len(list)})
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
		SELECT m.id, m.title, up.current_chapter, up.status, up.version
		FROM user_progress up
		LEFT JOIN manga m ON up.manga_id = m.id
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
		var title sql.NullString
		rows.Scan(&i.MangaID, &title, &i.CurrentChapter, &i.Status, &i.Version)
		if title.Valid {
			i.Title = title.String
		} else {
			i.Title = i.MangaID
		}
		items = append(items, i)
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

	// Validate max chapters if manga exists
	var totalChapters int
	if err := database.DB.QueryRow("SELECT total_chapters FROM manga WHERE id = ?", mangaID).Scan(&totalChapters); err == nil {
		if totalChapters > 0 && req.CurrentChapter > totalChapters {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Chapter %d exceeds total chapters (%d)", req.CurrentChapter, totalChapters)})
			return
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
		c.JSON(http.StatusNotFound, gin.H{"error": "Manga not in library"})
		return
	}

	// Fetch updated version
	var newVersion int
	database.DB.QueryRow("SELECT version FROM user_progress WHERE user_id = ? AND manga_id = ?", userID, mangaID).Scan(&newVersion)

	var title string
	database.DB.QueryRow("SELECT title FROM manga WHERE id = ?", mangaID).Scan(&title)

	// Broadcast via TCP to other sessions of the SAME user
	tcp.GlobalHub.BroadcastProgress(models.UserProgress{
		UserID:         userID,
		MangaID:        mangaID,
		CurrentChapter: req.CurrentChapter,
		Status:         req.Status,
		Version:        newVersion,
	}, username, title)

	c.JSON(http.StatusOK, gin.H{"message": "Progress updated", "version": newVersion})
}

// Legacy handler for PUT /users/progress
func updateProgressLegacyHandler(c *gin.Context) {
	userID := c.GetString("user_id")
	username := c.GetString("username")

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
	_ = userID
	_ = username
}
