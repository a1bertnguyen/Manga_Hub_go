package main

import (
	"bufio"
	"encoding/json"
	"log"
	"net"
	"strings"

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
	// Read first message for session authentication
	if !scanner.Scan() {
		log.Printf("[Backend TCP] Disconnected before authentication: %s", remoteAddr)
		return
	}

	firstMsg := strings.TrimSpace(scanner.Text())
	var userID string

	// Attempt JSON decode of auth payload: {"type":"auth","token":"..."} or {"user_id":"..."}
	var authPayload struct {
		Type   string `json:"type"`
		Token  string `json:"token"`
		UserID string `json:"user_id"`
	}
	if err := json.Unmarshal([]byte(firstMsg), &authPayload); err == nil {
		if authPayload.Token != "" {
			if claims, err := auth.ValidateToken(authPayload.Token); err == nil {
				userID = claims.UserID
			}
		}
		if userID == "" && authPayload.UserID != "" {
			userID = authPayload.UserID
		}
	}

	// Fallback to plain text UserID if not JSON
	if userID == "" {
		userID = firstMsg
	}

	if userID == "" {
		conn.Write([]byte("{\"type\":\"error\",\"message\":\"authentication_failed\"}\n"))
		return
	}

	client := &tcp.Client{
		Conn:   conn,
		UserID: userID,
		Send:   make(chan []byte, 256),
	}

	// Send auth confirmation newline frame
	authConfirm, _ := json.Marshal(map[string]interface{}{
		"type":    "authenticated",
		"user_id": userID,
		"status":  "ok",
	})
	conn.Write(append(authConfirm, '\n'))

	log.Printf("[Backend TCP] Session authenticated: %s -> UserID: %s", remoteAddr, userID)

	tcp.GlobalHub.Register <- client
	go client.WritePump()
	client.ReadPump()
}

