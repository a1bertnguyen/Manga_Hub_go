package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"net/http"

	"mangahub/internal/shared"
	"mangahub/internal/tcp"
	"mangahub/pkg/models"

	"github.com/gin-gonic/gin"
)

func main() {
	go tcp.GlobalHub.Run() // Start the global TCP hub in a separate goroutine

	go startTCPListener() // Start TCP listener on port 9090

	router := gin.New()
	router.POST("/internal/progress", receiveProgress) // Internal HTTP endpoint to receive progress updates

	log.Println("TCP Server running")
	log.Println(" - TCP clients on :9090")
	log.Println(" - Internal HTTP for API on :9091/internal/progress")

	router.Run(":9091") // Start HTTP server on port 9091
}

func startTCPListener() {
	listener, err := net.Listen("tcp", ":9090") // Open a TCP listener on port 9090
	if err != nil {
		log.Fatal("Error starting TCP listener:", err)
	}
	defer listener.Close() // closes when function exits

	log.Println("TCP listener started on :9090")

	for {
		conn, err := listener.Accept() // Wait for a new client connection
		if err != nil {
			log.Println("Error accepting connection:", err)
			continue
		}
		go handleTCPConnection(conn)
	}
}

func handleTCPConnection(conn net.Conn) {
	defer conn.Close() // Close connection when function returns

	remoteAddr := conn.RemoteAddr().String() // Get client IP and port
	log.Printf("TCP CLIENT CONNECTED: %s", remoteAddr)

	fmt.Fprintf(conn, "Welcome to MangaHub Progress!\nEnter your UserID: ")

	scanner := bufio.NewScanner(conn) // Scanner reads text input from TCP connection
	// Read UserID from client
	if !scanner.Scan() {
		log.Printf("TCP CLIENT DISCONNECTED (no UserID sent): %s", remoteAddr)
		return
	}

	userID := scanner.Text()
	log.Printf("TCP CLIENT AUTHENTICATED: %s → UserID: %s", remoteAddr, userID)

	// Create a new TCP client object
	client := &tcp.Client{
		Conn:   conn,
		UserID: userID,
		Send:   make(chan []byte, 256),
	}

	tcp.GlobalHub.Register <- client // Register client to the global hub
    // Start goroutine to send messages to client
	go client.WritePump()
	client.ReadPump() // Will trigger unregister on disconnect
}

// HTTP handler that receives manga progress updates
func receiveProgress(c *gin.Context) {
	var update shared.ProgressUpdate
	// Bind JSON request body to struct
	if err := c.ShouldBindJSON(&update); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("PROGRESS UPDATE RECEIVED → Broadcasting to TCP clients")
	log.Printf("   User: %s (ID: %s)", update.Username, update.UserID)
	log.Printf("   Manga: %s", update.MangaTitle)
	log.Printf("   Chapter: %d | Status: %s", update.CurrentChapter, update.Status)

	// Broadcast progress update to all connected TCP clients
	tcp.GlobalHub.BroadcastProgress(models.UserProgress{
		UserID:         update.UserID,
		MangaID:        update.MangaID,
		CurrentChapter: update.CurrentChapter,
		Status:         update.Status,
	}, update.Username, update.MangaTitle)

	// count number of clients
	clientCount := tcp.GlobalHub.GetClientCount()
	log.Printf("STREAMED UPDATE TO %d TCP CLIENT(S)", clientCount)

	c.JSON(http.StatusOK, gin.H{"status": "broadcasted"})
}
