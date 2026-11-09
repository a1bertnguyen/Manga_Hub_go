package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"mangahub/internal/websocket"

	"github.com/gin-gonic/gin"
	gorilla "github.com/gorilla/websocket" // avoid cònlict
)

// WebSocket upgrader configuration
var upgrader = gorilla.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

var hub *websocket.Hub // Global WebSocket hub instance

func main() {
	hub = websocket.NewHub()
	go hub.Run()

	router := gin.Default()

	// Simple CORS middleware (allows browser WebSocket connections)
	router.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		// Handle preflight requests
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	// Serve chat UI HTML file
	router.GET("/", func(c *gin.Context) {
		c.File(filepath.Join("web", "broadcast_chatroom.html"))
	})

	// WebSocket upgrade endpoint
	router.GET("/ws", handleWebSocket)

	// Server statistics endpoint
	router.GET("/stats", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"online_users": hub.GetClientCount(),
			"timestamp":    time.Now().Format("15:04:05"),
		})
	})

	fmt.Println("🚀 WebSocket Chat Server (Multiple Rooms) started on :9093")
	fmt.Println("📱 Open: http://localhost:9093")
	// Start HTTP server
	if err := router.Run(":9093"); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}

// Handles incoming WebSocket connection requests
func handleWebSocket(c *gin.Context) {
	username := c.Query("username")
	room := c.Query("room")
	// Default room if not provided
	if room == "" {
		room = "general"
	}
	if username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username required"})
		return
	}

	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("Upgrade error: %v", err)
		return
	}

	// Create a new WebSocket client
	client := &websocket.Client{
		Hub:      hub,
		Conn:     conn,
		Send:     make(chan []byte, 256),
		Username: username,
		Room:     room,
	}

	hub.Register <- client

	// Send recent message history for the room
	history := hub.GetMessageHistory(room)
	for _, msg := range history {
		data, _ := json.Marshal(msg)
		client.Send <- data
	}

	// Broadcast system "join" message
	joinMsg := websocket.Message{
		Type: "system",
		Text: fmt.Sprintf("%s joined the room", username),
		Time: time.Now().Format("15:04"),
		Room: room,
	}
	data, _ := json.Marshal(joinMsg)
	hub.Broadcast <- data

	go client.WritePump()
	go client.ReadPump()
}
