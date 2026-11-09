package tcp

import (
	"bufio"
	"time"
)

// reads messages from the TCP client
func (c *Client) ReadPump() {
	// Ensure client is unregistered and connection closed on exit
	defer func() {
		GlobalHub.Unregister <- c  // Remove client
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
	// Create a ticker to send heartbeat every 30 seconds
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		// Send application messages to the client
		case message, ok := <-c.Send:
			if !ok {
				c.Conn.Write([]byte("BYE\n"))
				return
			}
			c.Conn.Write(append(message, '\n'))
		// Periodic heartbeat to client
		case <-ticker.C:
			c.Conn.Write([]byte("PING\n"))
		}
	}
}