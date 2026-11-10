package tcp

import (
	"encoding/json"
	"log"
	"net"
	"sync"
	"time"

	"mangahub/pkg/models"
)

type Client struct {
	Conn   net.Conn
	UserID string
	Send   chan []byte
}

type UserBroadcast struct {
	UserID  string
	Message []byte
}

type Hub struct {
	clients    map[*Client]bool
	broadcast  chan UserBroadcast
	Register   chan *Client
	Unregister chan *Client
	mu         sync.RWMutex
}

var GlobalHub = NewHub()

// NewHub creates a new Hub instance for TCP progress synchronization
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan UserBroadcast, 256),
		Register:   make(chan *Client),
		Unregister: make(chan *Client),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.Register:
			h.mu.Lock()
			h.clients[client] = true
			count := len(h.clients)
			h.mu.Unlock()
			log.Printf("[TCP Hub] Client registered: UserID=%s from %s (Total clients: %d)",
				client.UserID, client.Conn.RemoteAddr().String(), count)

		case client := <-h.Unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.Send)
				log.Printf("[TCP Hub] Client disconnected: UserID=%s from %s (Remaining: %d)",
					client.UserID, client.Conn.RemoteAddr().String(), len(h.clients))
			}
			h.mu.Unlock()

		case ub := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				// User Isolation: only send to clients that belong to the SAME UserID
				if client.UserID == ub.UserID {
					select {
					case client.Send <- ub.Message:
					default:
						close(client.Send)
						delete(h.clients, client)
					}
				}
			}
			h.mu.RUnlock()
		}
	}
}

// BroadcastProgress sends a reading progress update ONLY to connected TCP clients of the same user.
// Framing: One JSON object per newline as required by the specification.
func (h *Hub) BroadcastProgress(update models.UserProgress, username, mangaTitle string) {
	msg := map[string]interface{}{
		"type":        "progress_updated",
		"user_id":     update.UserID,
		"username":    username,
		"manga_id":    update.MangaID,
		"manga_title": mangaTitle,
		"chapter":     update.CurrentChapter,
		"status":      update.Status,
		"version":     update.Version,
		"timestamp":   time.Now().Unix(),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[TCP Hub] Error marshaling progress update: %v", err)
		return
	}

	h.broadcast <- UserBroadcast{
		UserID:  update.UserID,
		Message: data,
	}
}

func (h *Hub) GetClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
