package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"mangahub/internal/auth"
	"mangahub/internal/database"
	"mangahub/internal/shared"
	"mangahub/pkg/models"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	_ "mangahub/docs"

	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

const tcpServerURL = "http://localhost:9091/internal/progress"
const udpServerURL = "http://localhost:9094/internal/progress"

// Request types for Swagger
type AddToLibraryRequest struct {
	MangaID string `json:"manga_id" binding:"required"`
	Status  string `json:"status" binding:"oneof=reading completed plan_to_read"`
}

type UpdateProgressRequest struct {
	MangaID        string `json:"manga_id" binding:"required"`
	CurrentChapter int    `json:"current_chapter" binding:"gte=0"`
	Status         string `json:"status" binding:"omitempty,oneof=reading completed plan_to_read"`
}

func main() {
	if err := database.Initialize("./data/mangahub.db"); err != nil {
		log.Fatal("Failed to initialize database:", err)
	}
	defer database.Close()

	if err := database.SeedManga(); err != nil {
		log.Printf("Warning: Failed to seed manga data: %v", err)
	}

	router := gin.Default()

	// Swagger endpoint
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	router.Use(cors.Default())
	// router.Use(cors.New(cors.Config{
	// 	//AllowOrigins:     []string{"http://127.0.0.1:5500", "http://localhost:5500", "http://localhost:8080"},
	// 	AllowAllOrigins:  true,
	// 	AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
	// 	AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
	// 	ExposeHeaders:    []string{"Content-Length"},
	// 	AllowCredentials: true,
	// 	MaxAge:           12 * time.Hour,
	// }))

	router.GET("/", func(c *gin.Context) {
		c.File(filepath.Join("web", "search_manga.html"))
	})

	public := router.Group("/")
	{
		public.POST("/auth/register", registerHandler)
		public.POST("/auth/login", loginHandler)
	}

	protected := router.Group("/")
	protected.Use(auth.Middleware())
	{
		protected.GET("/manga", getMangaHandler)
		protected.GET("/manga/:id", getMangaDetailHandler)
		protected.POST("/manga", createMangaHandler)
		protected.POST("/users/library", addToLibraryHandler)
		protected.GET("/users/library", getLibraryHandler)
		protected.PUT("/users/progress", updateProgressHandler)
	}

	log.Println("API Server starting on http://localhost:8080")
	log.Println("Swagger docs: http://localhost:8080/swagger/index.html")

	// Serve static web files
	router.Static("/web", "./web")
	router.StaticFS("/static", http.Dir("./web"))

	// Serve search_manga.html as the main page at root /
	router.GET("/search_manga.html", func(c *gin.Context) {
		c.File("./web/search_manga.html")
	})

	router.Run(":8080")
}

// Register new user
// @Summary      Register a new user
// @Description  Create a new user account
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        request body models.RegisterRequest true "User registration details"
// @Success      200 {object} models.LoginResponse "User created and JWT token returned"
// @Failure      400 {object} map[string]string "Invalid request"
// @Failure      500 {object} map[string]string "Server error"
// @Router       /auth/register [post]
func registerHandler(c *gin.Context) {
	var req models.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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

// Login user
// @Summary      Login user
// @Description  Authenticate user and return JWT token
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        request body models.LoginRequest true "Login credentials"
// @Success      200 {object} models.LoginResponse "JWT token returned"
// @Failure      400 {object} map[string]string "Invalid request"
// @Failure      401 {object} map[string]string "Invalid credentials"
// @Failure      500 {object} map[string]string "Server error"
// @Router       /auth/login [post]
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

// Get manga detail
// @Summary      Get manga by ID
// @Description  Retrieve detailed information about a specific manga
// @Tags         Manga
// @Produce      json
// @Param        id path string true "Manga ID"
// @Param        Authorization header string true "Bearer {token}"
// @Success      200 {object} models.Manga
// @Failure      404 {object} map[string]string "Manga not found"
// @Failure      500 {object} map[string]string "Server error"
// @Router       /manga/{id} [get]
func getMangaDetailHandler(c *gin.Context) {
	id := c.Param("id")
	var m models.Manga
	err := database.DB.QueryRow(`SELECT id, title, author, genres, status, total_chapters, description FROM manga WHERE id = ?`, id).
		Scan(&m.ID, &m.Title, &m.Author, &m.GenresString, &m.Status, &m.TotalChapters, &m.Description)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Manga not found"})
		return
	} else if err != nil {
		log.Printf("Database error fetching manga detail: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	m.PostScan()
	c.JSON(http.StatusOK, m)
}

// Search or list manga
// @Summary      Search or list all manga
// @Description  Search manga by title (?search= or ?title=) or return all if no query
// @Tags         Manga
// @Produce      json
// @Param        search query string false "Search term (title)"
// @Param        title  query string false "Alternative search term"
// @Param        Authorization header string true "Bearer {token}"
// @Success      200 {object} map[string]any "List of manga with count"
// @Failure      500 {object} map[string]string "Server error"
// @Router       /manga [get]
func getMangaHandler(c *gin.Context) {
	query := c.Query("search")
	if query == "" {
		query = c.Query("title")
	}

	var mangaList []models.Manga
	var rows *sql.Rows
	var err error

	if query != "" {
		searchTerm := "%" + query + "%"
		rows, err = database.DB.Query(`
			SELECT id, title, author, genres, status, total_chapters, description 
			FROM manga 
			WHERE LOWER(title) LIKE LOWER(?)
			ORDER BY title
		`, searchTerm)
	} else {
		rows, err = database.DB.Query(`
			SELECT id, title, author, genres, status, total_chapters, description 
			FROM manga 
			ORDER BY title
		`)
	}

	if err != nil {
		log.Printf("Database error fetching manga: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch manga"})
		return
	}
	defer rows.Close()

	for rows.Next() {
		var m models.Manga
		err := rows.Scan(&m.ID, &m.Title, &m.Author, &m.GenresString, &m.Status, &m.TotalChapters, &m.Description)
		if err != nil {
			log.Printf("Scan error: %v", err)
			continue
		}
		m.PostScan()
		mangaList = append(mangaList, m)
	}

	c.JSON(http.StatusOK, gin.H{
		"manga": mangaList,
		"count": len(mangaList),
	})
}

// Create new manga (admin only in real app)
// @Summary      Create new manga
// @Description  Add a new manga to the catalog
// @Tags         Manga
// @Accept       json
// @Produce      json
// @Param        manga body models.Manga true "Manga data"
// @Param        Authorization header string true "Bearer {token}"
// @Success      201 {object} models.Manga
// @Failure      400 {object} map[string]string "Invalid input"
// @Failure      500 {object} map[string]string "Server error"
// @Router       /manga [post]
func createMangaHandler(c *gin.Context) {
	var m models.Manga
	if err := c.ShouldBindJSON(&m); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	m.ID = auth.GenerateID("mng")
	m.PreSave()

	_, err := database.DB.Exec(
		"INSERT INTO manga (id, title, author, genres, status, total_chapters, description) VALUES (?, ?, ?, ?, ?, ?, ?)",
		m.ID, m.Title, m.Author, m.GenresString, m.Status, m.TotalChapters, m.Description,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create manga"})
		return
	}

	c.JSON(http.StatusCreated, m)
}

// Add manga to user library
// @Summary      Add manga to library
// @Description  Add a manga to the authenticated user's library with optional status
// @Tags         Library
// @Accept       json
// @Produce      json
// @Param        Authorization header string true "Bearer {token}"
// @Param        request body AddToLibraryRequest true "Manga ID and optional status"
// @Success      200 {object} map[string]string "Added to library"
// @Failure      400 {object} map[string]string "Invalid input"
// @Failure      404 {object} map[string]string "Manga not found"
// @Failure      500 {object} map[string]string "Server error"
// @Router       /users/library [post]
func addToLibraryHandler(c *gin.Context) {
	userID := c.GetString("user_id")

	var req AddToLibraryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var exists int
	err := database.DB.QueryRow("SELECT COUNT(*) FROM manga WHERE id = ?", req.MangaID).Scan(&exists)
	if err != nil || exists == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Manga not found"})
		return
	}

	status := req.Status
	if status == "" {
		status = "reading"
	}

	_, err = database.DB.Exec(`
		INSERT INTO user_progress (user_id, manga_id, current_chapter, status)
		VALUES (?, ?, 0, ?)
		ON CONFLICT(user_id, manga_id) DO UPDATE SET status = excluded.status
	`, userID, req.MangaID, status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add to library"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Added to library"})
}

// Get user library
// @Summary      Get user library
// @Description  Retrieve all manga in the authenticated user's library
// @Tags         Library
// @Produce      json
// @Param        Authorization header string true "Bearer {token}"
// @Success      200 {object} map[string]any "Library with count"
// @Failure      500 {object} map[string]string "Server error"
// @Router       /users/library [get]
func getLibraryHandler(c *gin.Context) {
	userID := c.GetString("user_id")

	rows, err := database.DB.Query(`
		SELECT m.id, m.title, up.current_chapter, up.status
		FROM user_progress up
		JOIN manga m ON up.manga_id = m.id
		WHERE up.user_id = ?
	`, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch library"})
		return
	}
	defer rows.Close()

	type item struct {
		MangaID        string `json:"manga_id"`
		Title          string `json:"title"`
		CurrentChapter int    `json:"current_chapter"`
		Status         string `json:"status"`
	}
	var library []item
	for rows.Next() {
		var i item
		rows.Scan(&i.MangaID, &i.Title, &i.CurrentChapter, &i.Status)
		library = append(library, i)
	}
	c.JSON(http.StatusOK, gin.H{"library": library, "count": len(library)})
}

// Update reading progress
// @Summary      Update reading progress
// @Description  Update current chapter and optional status. Triggers broadcast to TCP.
// @Tags         Progress
// @Accept       json
// @Produce      json
// @Param        Authorization header string true "Bearer {token}"
// @Param        request body UpdateProgressRequest true "Progress update data"
// @Success      200 {object} map[string]string "Progress updated and broadcasted"
// @Failure      400 {object} map[string]string "Invalid input"
// @Failure      500 {object} map[string]string "Server error"
// @Router       /users/progress [put]
func updateProgressHandler(c *gin.Context) {
	userID := c.GetString("user_id")
	username := c.GetString("username")

	var req UpdateProgressRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	_, err := database.DB.Exec(`
		UPDATE user_progress 
		SET current_chapter = ?, updated_at = CURRENT_TIMESTAMP
		WHERE user_id = ? AND manga_id = ?
	`, req.CurrentChapter, userID, req.MangaID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update progress"})
		return
	}

	if req.Status != "" {
		_, err = database.DB.Exec(`
			UPDATE user_progress SET status = ? WHERE user_id = ? AND manga_id = ?
		`, req.Status, userID, req.MangaID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update status"})
			return
		}
	}

	var mangaTitle string
	if err := database.DB.QueryRow("SELECT title FROM manga WHERE id = ?", req.MangaID).Scan(&mangaTitle); err != nil {
		mangaTitle = "Unknown Manga"
	}

	if username == "" {
		database.DB.QueryRow("SELECT username FROM users WHERE id = ?", userID).Scan(&username)
		username = "Unknown User"
	}

	payload := shared.ProgressUpdate{
		UserID:         userID,
		Username:       username,
		MangaID:        req.MangaID,
		MangaTitle:     mangaTitle,
		CurrentChapter: req.CurrentChapter,
		Status:         req.Status,
		Timestamp:      time.Now().Unix(),
	}

	jsonData, _ := json.Marshal(payload)

	go func() {
		resp, err := http.Post(tcpServerURL, "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			log.Printf("FAILED to broadcast to TCP server: %v", err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			log.Printf("TCP server responded with status: %d", resp.StatusCode)
		}
	}()

	go func() {
		resp, err := http.Post(udpServerURL, "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			log.Printf("FAILED to broadcast to UDP server: %v", err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			log.Printf("UDP server responded with status: %d", resp.StatusCode)
		}
	}()

	c.JSON(http.StatusOK, gin.H{"message": "Progress updated and broadcasted"})
}
