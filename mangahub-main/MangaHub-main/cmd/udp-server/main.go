package main

import (
	"log"
	"net/http"

	"mangahub/internal/shared"
	"mangahub/internal/udp"
	"mangahub/pkg/models"

	"github.com/gin-gonic/gin"
)

func main() {
	go udp.GlobalHub.Run() // Start the global UDP hub

	udp.StartUDPListener(":9091") // UDP listener on :9091

	router := gin.New()
	router.POST("/internal/progress", receiveProgress)

	log.Println("UDP Server running")
	log.Println(" - UDP clients on :9091")
	log.Println(" - Internal HTTP trigger on :9094")

	if err := router.Run(":9094"); err != nil { // <-- Add this to start HTTP on :9094
		log.Fatal("Failed to start UDP HTTP trigger:", err)
	}
}

// receiveProgress is unchanged, but now it's served
func receiveProgress(c *gin.Context) {
	var update shared.ProgressUpdate
	if err := c.ShouldBindJSON(&update); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("BROADCAST NOTIFICATION RECEIVED → Sending to UDP subscribers")
	log.Printf("   User: %s (ID: %s)", update.Username, update.UserID)
	log.Printf("   Manga: %s → Chapter %d (%s)", update.MangaTitle, update.CurrentChapter, update.Status)

	udp.GlobalHub.BroadcastProgress(models.UserProgress{
		UserID:         update.UserID,
		MangaID:        update.MangaID,
		CurrentChapter: update.CurrentChapter,
		Status:         update.Status,
	}, update.Username, update.MangaTitle)

	clientCount := udp.GlobalHub.GetClientCount()
	log.Printf("BROADCAST SENT TO %d UDP SUBSCRIBER(S)", clientCount)

	c.JSON(http.StatusOK, gin.H{"status": "broadcasted"})
}
