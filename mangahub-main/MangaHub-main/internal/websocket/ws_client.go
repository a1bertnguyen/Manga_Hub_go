package websocket

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"strings"
	"time"

	"mangahub/pkg/models"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = 54 * time.Second
	maxMessageSize = models.MaxChatFrameBytes
)

// ReadPump reads messages from the WebSocket client
func (c *Client) ReadPump() {
	defer func() {
		c.Hub.Unregister <- c // Remove client
		c.Conn.Close()
	}()
	if c.Registered != nil {
		<-c.Registered // Replay snapshot and room membership precede incoming writes.
	}

	c.Conn.SetReadLimit(maxMessageSize)              // Limit size of incoming messages
	c.Conn.SetReadDeadline(time.Now().Add(pongWait)) // Set initial read deadline (connection timeout)
	// Reset read deadline every time a pong is received
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, data, err := c.Conn.ReadMessage()
		if err != nil {
			break
		}

		var msg Message
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		if msg.Type != "chat" || strings.TrimSpace(msg.Text) == "" {
			continue
		}

		// Set server-side fields
		msg.Username = c.Username
		msg.UserID = c.UserID
		now := time.Now()
		msg.Time = now.Format("15:04")
		msg.CreatedAt = now.UTC().Format(time.RFC3339Nano)
		msg.Room = c.Room
		if msg.ID == "" {
			msg.ID = rand.Text()
		}
		if c.Hub.store != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			err := c.Hub.store.Save(ctx, msg)
			cancel()
			if err != nil {
				failure, _ := json.Marshal(Message{ID: msg.ID, Type: "error", Room: c.Room, Text: "Message could not be saved; text kept for retry"})
				if !c.Hub.sendToClient(c, failure) {
					return
				}
				continue // No broadcast or successful echo on a failed SQLite write.
			}
		}

		// Re-marshal the message with updated fields
		messageData, err := json.Marshal(msg)
		if err != nil {
			continue
		}

		// Broadcast the properly formatted message
		c.Hub.Broadcast <- messageData
	}
}

// WritePump sends messages and heartbeats to the WebSocket client
func (c *Client) WritePump() {
	// Ticker sends periodic ping messages
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// Channel closed, tell client to close connection
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			c.Conn.WriteMessage(websocket.TextMessage, message)

		// Send periodic ping to keep connection alive
		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			// If ping fails, client is considered disconnected
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
