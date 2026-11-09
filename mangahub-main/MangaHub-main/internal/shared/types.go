package shared

import "time"

// ProgressUpdate is the message format sent to TCP clients when reading progress changes
type ProgressUpdate struct {
	UserID         string `json:"user_id"`
	Username       string `json:"username"`
	MangaID        string `json:"manga_id"`
	MangaTitle     string `json:"manga_title"`
	CurrentChapter int    `json:"current_chapter"`
	Status         string `json:"status"`
	Timestamp      int64  `json:"timestamp"` // Unix timestamp
	Type           string `json:"type"`
}

// Helper to create a new update
func NewProgressUpdate(userID, username, mangaID, mangaTitle string, chapter int, status string) ProgressUpdate {
	return ProgressUpdate{
		UserID:         userID,
		Username:       username,
		MangaID:        mangaID,
		MangaTitle:     mangaTitle,
		CurrentChapter: chapter,
		Status:         status,
		Timestamp:      time.Now().Unix(),
	}
}
