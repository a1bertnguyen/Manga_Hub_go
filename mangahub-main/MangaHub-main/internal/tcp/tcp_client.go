package tcp

import (
	"bufio"
	"time"
)

// reads messages from the TCP client
func (c *Client) ReadPump() {
	// Ensure client is unregistered and connection closed on exit
	defer func() {
		GlobalHub.Unregister <- c // Remove client
		c.Conn.Close()
	}()

	// Scanner reads input line-by-line
	scanner := bufio.NewScanner(c.Conn)
	for scanner.Scan() {
		line := scanner.Text()
		// Handle heartbeat response from client
		if line == "PING" {
			c.Conn.Write([]byte("PONG\n")) // Reply to keep connection alive
		}
	}
}

// sends messages to the TCP client
func (c *Client) WritePump() {
	defer c.Conn.Close()
	// Create a ticker to send heartbeat every 30 seconds
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		// Send application messages to the client
		case message, ok := <-c.Send:
			if !ok {
				return
			}
			_ = c.Conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if _, err := c.Conn.Write(append(message, '\n')); err != nil {
				return
			}
		// Periodic heartbeat to client
		case <-ticker.C:
			_ = c.Conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if _, err := c.Conn.Write([]byte("PING\n")); err != nil {
				return
			}
		}
	}
}
