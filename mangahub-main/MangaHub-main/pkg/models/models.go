package models

import (
	"time"
    "strings"
)


// User represents a user account
type User struct {
	ID           string    `json:"id" db:"id"`
	Username     string    `json:"username" db:"username"`
	Email        string    `json:"email" db:"email"`
	PasswordHash string    `json:"-" db:"password_hash"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

// Manga represents a manga series
type Manga struct {
	ID            string   `json:"id" db:"id"`
	Title         string   `json:"title" db:"title"`
	Author        string   `json:"author" db:"author"`
	Genres        []string `json:"genres"`
	GenresString  string   `json:"-" db:"genres"` // For DB storage
	Status        string   `json:"status" db:"status"`
	TotalChapters int      `json:"total_chapters" db:"total_chapters"`
	Description   string   `json:"description" db:"description"`
}

// PostScan processes the Manga struct after scanning from the database.
// It converts the genres string (comma-separated) into a slice of strings.
// This method should be called after sql.Rows.Scan() on a Manga struct.
func (m *Manga) PostScan() {
	if m.GenresString != "" {
		m.Genres = strings.Split(m.GenresString, ",")
	}
}

// PreSave prepares the Manga struct before saving to the database.
// To save to DB, convert []string to string
func (m *Manga) PreSave() {
	if len(m.Genres) > 0 {
		m.GenresString = strings.Join(m.Genres, ",")
	}
}

// UserProgress tracks reading progress
type UserProgress struct {
	UserID         string    `json:"user_id" db:"user_id"`
	MangaID        string    `json:"manga_id" db:"manga_id"`
	CurrentChapter int       `json:"current_chapter" db:"current_chapter"`
	Status         string    `json:"status" db:"status"`
	UpdatedAt      time.Time `json:"updated_at" db:"updated_at"`
}

// LoginRequest for user login
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// RegisterRequest for user registration
type RegisterRequest struct {
	Username string `json:"username" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
}

// LoginResponse after successful login
type LoginResponse struct {
	Token    string `json:"token"`
	Username string `json:"username"`
	UserID   string `json:"user_id"`
}