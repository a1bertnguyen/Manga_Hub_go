package websocket

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/gorilla/websocket" 
)

// Message represents a chat or system message sent over WebSocket
type Message struct {
	Type     string `json:"type"`              // Message type: chat, system, etc.
	Username string `json:"username"`          // Sender username
	Text     string `json:"text"`              // Message content
	Time     string `json:"time"`              // Display time (HH:MM)
	Room     string `json:"room,omitempty"`    // Chat room (optional in JSON)
}

// Client represents a single WebSocket connection
type Client struct {
	Hub      *Hub              // Reference to the central hub
	Conn     *websocket.Conn   // WebSocket connection
	Send     chan []byte       // Outgoing message channel
	Username string            // Client username
	Room     string            // Room the client joined
}

// Hub manages all WebSocket clients and rooms
type Hub struct {
	clients    map[*Client]bool              // All connected clients
	rooms      map[string]map[*Client]bool   // Room → clients mapping
	Broadcast  chan []byte                   // Messages to broadcast
	Register   chan *Client                  // New client registration
	Unregister chan *Client                  // Client disconnection
	mu         sync.RWMutex                  // Protects clients & rooms

	messageHistory []Message                 // Last 50 messages (all rooms)
	historyMu      sync.RWMutex              // Protects message history
}

// Creates and initializes a new Hub instance
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),          // Initialize client map
		rooms:      make(map[string]map[*Client]bool), // Initialize rooms
		Broadcast:  make(chan []byte, 256),           // Buffered broadcast channel
		Register:   make(chan *Client),               // Register channel
		Unregister: make(chan *Client),               // Unregister channel
		messageHistory: make([]Message, 0, 50),       // Pre-allocate history
	}
}

// Main event loop of the Hub
func (h *Hub) Run() {
	for {
		select {

		// Handle new client connection
		case client := <-h.Register:
			h.mu.Lock()
			h.clients[client] = true

			// Create room if it doesn't exist
			if h.rooms[client.Room] == nil {
				h.rooms[client.Room] = make(map[*Client]bool)
			}

			// Add client to room
			h.rooms[client.Room][client] = true
			h.mu.Unlock()

		// Handle client disconnection
		case client := <-h.Unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.Send)

				// Remove client from its room
				if roomClients, exists := h.rooms[client.Room]; exists {
					delete(roomClients, client)
					if len(roomClients) == 0 {
						delete(h.rooms, client.Room)
					}
				}
			}
			h.mu.Unlock()

			// Broadcast system "leave" message
			leaveMsg := Message{
				Type: "system",
				Text: client.Username + " left the room",
				Time: time.Now().Format("15:04"),
				Room: client.Room,
			}
			data, _ := json.Marshal(leaveMsg)
			h.Broadcast <- data

		// Handle incoming broadcast messages
		case data := <-h.Broadcast:
			var msg Message

			// Decode message to inspect its content
			if err := json.Unmarshal(data, &msg); err != nil {
				continue
			}

			// Save chat & system messages to history
			if msg.Type == "chat" || msg.Type == "system" {
				h.historyMu.Lock()
				h.messageHistory = append(h.messageHistory, msg)

				// Keep only last 50 messages
				if len(h.messageHistory) > 50 {
					h.messageHistory = h.messageHistory[1:]
				}
				h.historyMu.Unlock()
			}

			// Send message to clients in the same room
			h.mu.RLock()
			if roomClients, exists := h.rooms[msg.Room]; exists {
				for client := range roomClients {
					select {
					case client.Send <- data:
						// Message sent successfully
					default:
						// Client send buffer full → disconnect
						close(client.Send)
						delete(h.clients, client)
						delete(roomClients, client)
					}
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Returns message history for a specific room
func (h *Hub) GetMessageHistory(room string) []Message {
	h.historyMu.RLock()
	defer h.historyMu.RUnlock()

	var history []Message
	for _, msg := range h.messageHistory {
		if msg.Room == room {
			history = append(history, msg)
		}
	}
	return history
}

// Returns total number of connected WebSocket clients
func (h *Hub) GetClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}