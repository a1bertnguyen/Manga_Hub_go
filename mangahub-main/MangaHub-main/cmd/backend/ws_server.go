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
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// handleWebSocketUpgrade upgrades HTTP connection to WebSocket and assigns client to discussion room
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

