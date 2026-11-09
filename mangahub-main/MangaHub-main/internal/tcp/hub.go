package tcp

import (
	"encoding/json"
	"log"
	"net"
	"sync"
	"time"

	"mangahub/internal/shared"
	"mangahub/pkg/models"
)

type Client struct {
	Conn   net.Conn
	UserID string
	Send   chan []byte
}

type Hub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	Register   chan *Client
	Unregister chan *Client
	mu         sync.RWMutex
}

var GlobalHub = &Hub{
	clients:    make(map[*Client]bool),
	broadcast:  make(chan []byte),
	Register:   make(chan *Client),
	Unregister: make(chan *Client),
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.Register:
			h.mu.Lock()
			h.clients[client] = true
			count := len(h.clients)
			h.mu.Unlock()
			log.Printf("TCP CLIENT REGISTERED IN HUB: UserID %s from %s — Total clients: %d", client.UserID, client.Conn.RemoteAddr().String(), count)

		case client := <-h.Unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.Send)
				log.Printf("TCP CLIENT DISCONNECTED: UserID %s from %s — Remaining clients: %d",
					client.UserID, client.Conn.RemoteAddr(), len(h.clients))
			}
			h.mu.Unlock()

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.Send <- message:
				default:
					close(client.Send)
					delete(h.clients, client)
				}
			}
			h.mu.RUnlock()
		}
	}
}

// BroadcastProgress sends a reading progress update to all connected TCP clients.
// Parameters:
//   update      - the user's progress data (chapter, status)
//   username    - display name of the user
//   mangaTitle  - title of the manga being read
// This is used when a user updates their reading progress via the API.
func (h *Hub) BroadcastProgress(update models.UserProgress, username, mangaTitle string) {
	msg := shared.ProgressUpdate{
		UserID:         update.UserID,
		Username:       username,
		MangaID:        update.MangaID,
		MangaTitle:     mangaTitle,
		CurrentChapter: update.CurrentChapter,
		Status:         update.Status,
		Timestamp:      time.Now().Unix(),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		log.Println("Error marshaling progress update:", err)
		return
	}

	h.broadcast <- data
}

func (h *Hub) GetClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
