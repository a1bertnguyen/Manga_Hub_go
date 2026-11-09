package main

import (
	"log"
	"net"
	"time"
)

func main() {
	// Connect to server
	serverAddr, err := net.ResolveUDPAddr("udp", "localhost:9091")
	if err != nil {
		log.Fatal("Resolve error:", err)
	}

	conn, err := net.DialUDP("udp", nil, serverAddr)
	if err != nil {
		log.Fatal("Dial error:", err)
	}
	defer conn.Close()

	// Periodically send PING to stay subscribed
	go func() {
		for {
			_, err := conn.Write([]byte("PING\n"))
			if err != nil {
				log.Println("Failed to send PING:", err)
				return
			}
			log.Println("Sent PING to server")
			time.Sleep(20 * time.Second)
		}
	}()

	log.Println("UDP client started. Waiting for notifications from server...")

	// Buffer for incoming notifications
	buffer := make([]byte, 2048)

	for {
		// Correct way to read from UDP: use ReadFromUDP
		n, server, err := conn.ReadFromUDP(buffer)
		if err != nil {
			log.Println("Read error:", err)
			continue
		}

		message := string(buffer[:n])
		if message == "PONG\n" {
			log.Println("Received PONG from server")
		} else {
			log.Printf("Notification from %s: %s", server, message)
		}
	}
}
