package udp

import (
	"encoding/json"
	"log"
	"net"
	"sync"
	"time"

	"mangahub/internal/shared"
	"mangahub/pkg/models"
)

type ClientAddr struct {
	Addr     *net.UDPAddr
	LastSeen time.Time // Last activity time (heartbeat)
}

// Hub manages all UDP subscribers and broadcasts
type Hub struct {
	clients    map[string]*ClientAddr
	broadcast  chan []byte
	Register   chan *ClientAddr
	Unregister chan *ClientAddr
	mu         sync.RWMutex
}

var GlobalHub = NewHub()

// NewHub creates a new Hub instance for UDP notification management
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[string]*ClientAddr),
		broadcast:  make(chan []byte, 256),
		Register:   make(chan *ClientAddr),
		Unregister: make(chan *ClientAddr),
	}
}

func (h *Hub) Run() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case client := <-h.Register:
			key := client.Addr.String()
			h.mu.Lock()
			if old, exists := h.clients[key]; exists {
				old.LastSeen = time.Now()
			} else {
				h.clients[key] = client
				log.Printf("[UDP Hub] Subscriber registered: %s (Total: %d)", key, len(h.clients))
			}
			h.mu.Unlock()

		case client := <-h.Unregister:
			key := client.Addr.String()
			h.mu.Lock()
			if _, exists := h.clients[key]; exists {
				delete(h.clients, key)
				log.Printf("[UDP Hub] Subscriber unregistered: %s (Remaining: %d)", key, len(h.clients))
			}
			h.mu.Unlock()

		case message := <-h.broadcast:
			h.mu.RLock()
			for key, client := range h.clients {
				if udpConn != nil {
					_, err := udpConn.WriteToUDP(message, client.Addr)
					if err != nil {
						log.Printf("[UDP Hub] UDP send failed to %s: %v", key, err)
					} else {
						client.LastSeen = time.Now()
					}
				}
			}
			h.mu.RUnlock()

		case <-ticker.C:
			h.mu.Lock()
			now := time.Now()
			for key, client := range h.clients {
				if now.Sub(client.LastSeen) > 60*time.Second {
					delete(h.clients, key)
					log.Printf("[UDP Hub] Subscriber timed out: %s (Remaining: %d)", key, len(h.clients))
				}
			}
			h.mu.Unlock()
		}
	}
}

// BroadcastChapterRelease broadcasts a new chapter release notification to all registered UDP subscribers.
// Message format matches the specification: {"type":"new_chapter","manga_id":"one-piece","chapter":26,...}
func (h *Hub) BroadcastChapterRelease(mangaID, mangaTitle string, chapter int) {
	msg := map[string]interface{}{
		"type":        "new_chapter",
		"manga_id":    mangaID,
		"manga_title": mangaTitle,
		"chapter":     chapter,
		"timestamp":   time.Now().Unix(),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[UDP Hub] Marshal error: %v", err)
		return
	}

	log.Printf("[UDP Hub] Broadcasting new chapter alert to %d subscribers: %s Ch.%d", h.GetClientCount(), mangaTitle, chapter)
	h.broadcast <- append(data, '\n')
}

func (h *Hub) BroadcastProgress(update models.UserProgress, username, mangaTitle string) {
	msg := shared.ProgressUpdate{
		UserID:         update.UserID,
		Username:       username,
		MangaID:        update.MangaID,
		MangaTitle:     mangaTitle,
		CurrentChapter: update.CurrentChapter,
		Status:         update.Status,
		Timestamp:      time.Now().Unix(),
		Type:           "progress_update",
	}

	data, err := json.Marshal(msg)
	if err != nil {
		log.Println("UDP marshal error:", err)
		return
	}

	h.broadcast <- append(data, '\n')
}

func (h *Hub) GetClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
