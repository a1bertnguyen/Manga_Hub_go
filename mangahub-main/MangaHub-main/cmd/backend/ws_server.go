package main

import (
	"log"
	"net/http"

	wsHub "mangahub/internal/websocket"

	"github.com/gin-gonic/gin"
	gorilla "github.com/gorilla/websocket"
)

var upgrader = gorilla.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// handleWebSocketUpgrade upgrades HTTP connection to WebSocket and assigns client to discussion room
func handleWebSocketUpgrade(c *gin.Context) {
	username := c.GetString("username")
	if c.GetString("user_id") == "" || username == "" {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}
	room := c.Query("room")
	if room == "" {
		room = "general"
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
