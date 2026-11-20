package main

import (
	"bufio"
	"crypto/rand"
	"encoding/json"
	"log"
	"net"
	"strings"
	"time"

	"mangahub/internal/auth"
	"mangahub/internal/tcp"
)

// startTCPListener binds and listens on the TCP port for progress sync clients
func startTCPListener(port string) {
	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Printf("[Backend TCP] Failed to listen on :%s: %v", port, err)
		return
	}
	defer listener.Close()

	log.Printf("[Backend TCP] Progress sync server listening on :%s", port)
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("[Backend TCP] Error accepting connection: %v", err)
			continue
		}
		go handleTCPConnection(conn)
	}
}

// handleTCPConnection manages a client session over TCP.
// Message Framing: One JSON object per newline. First message authenticates the session.
func handleTCPConnection(conn net.Conn) {
	defer conn.Close()

	remoteAddr := conn.RemoteAddr().String()
	log.Printf("[Backend TCP] Client connected: %s", remoteAddr)

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 1024), 16*1024)
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_ = conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	// Read first message for session authentication
	if !scanner.Scan() {
		log.Printf("[Backend TCP] Disconnected before authentication: %s", remoteAddr)
		return
	}

	firstMsg := strings.TrimSpace(scanner.Text())
	// Identity comes exclusively from a verified JWT, never a client-supplied ID.
	var authPayload struct {
		Type  string `json:"type"`
		Token string `json:"token"`
	}
	if err := json.Unmarshal([]byte(firstMsg), &authPayload); err != nil || authPayload.Type != "auth" || authPayload.Token == "" {
		conn.Write([]byte("{\"type\":\"error\",\"message\":\"authentication_failed\"}\n"))
		return
	}
	claims, err := auth.ValidateToken(authPayload.Token)
	if err != nil {
		conn.Write([]byte("{\"type\":\"error\",\"message\":\"authentication_failed\"}\n"))
		return
	}
	userID := claims.UserID
	_ = conn.SetReadDeadline(time.Time{})
	_ = conn.SetWriteDeadline(time.Time{})

	client := &tcp.Client{
		Conn:       conn,
		UserID:     userID,
		SessionID:  rand.Text(),
		Send:       make(chan []byte, 256),
		Registered: make(chan struct{}),
	}

	// Send auth confirmation newline frame
	authConfirm, _ := json.Marshal(map[string]interface{}{
		"type":       "authenticated",
		"user_id":    userID,
		"session_id": client.SessionID,
		"status":     "ok",
	})
	client.Send <- authConfirm

	log.Printf("[Backend TCP] Session authenticated: %s -> UserID: %s", remoteAddr, userID)

	tcp.GlobalHub.Register <- client
	// The auth acknowledgement must not reach the TUI before HTTP can validate its session.
	<-client.Registered
	go client.WritePump()
	client.ReadPump()
}
