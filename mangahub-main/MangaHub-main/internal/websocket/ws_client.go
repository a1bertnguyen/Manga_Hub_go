package websocket

import (
	"encoding/json"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = 54 * time.Second
	maxMessageSize = 512
)

// ReadPump reads messages from the WebSocket client
func (c *Client) ReadPump() {
	defer func() {
		c.Hub.Unregister <- c  // Remove client
		c.Conn.Close()
	}()

	c.Conn.SetReadLimit(maxMessageSize) // Limit size of incoming messages 
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

		// Set server-side fields
		msg.Username = c.Username
		msg.Time = time.Now().Format("15:04")
		msg.Room = c.Room

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
