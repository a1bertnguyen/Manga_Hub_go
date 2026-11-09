package udp

import (
	"log"
	"net"
	"strings"
	"time"
)

var udpConn *net.UDPConn // Shared UDP connection used for sending and receiving packets

func StartUDPListener(addr string) {
	// Resolve string address into UDP address structure
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		log.Fatal("UDP resolve error:", err)
	}

    // Open UDP socket and start listening
	udpConn, err = net.ListenUDP("udp", udpAddr)
	if err != nil {
		log.Fatal("UDP listen error:", err)
	}
	log.Printf("UDP notification server running on %s", addr)

	go readPump() // udp receive
	go writePump() // udp send
}

// reads incoming UDP packets
func readPump() {
	buffer := make([]byte, 4096)
	for {
		// Read data from any UDP client
		n, clientAddr, err := udpConn.ReadFromUDP(buffer)
		if err != nil {
			log.Println("UDP read error:", err)
			continue
		}

		// Convert received bytes into trimmed string
		message := strings.TrimSpace(string(buffer[:n]))
		addrStr := clientAddr.String()

		if message == "PING" {
			log.Printf("UDP PING RECEIVED from %s → Sending PONG", addrStr)

			// Register or refresh UDP client in hub
			client := &ClientAddr{
				Addr:     clientAddr,
				LastSeen: time.Now(),
			}
			GlobalHub.Register <- client

			// Reply to client to confirm subscription
			udpConn.WriteToUDP([]byte("PONG\n"), clientAddr)
			log.Printf("UDP CLIENT SUBSCRIBED: %s — Total subscribers: %d", addrStr, GlobalHub.GetClientCount())
		}
	}
}

// Sends broadcast messages to all registered UDP clients
func writePump() {
	for message := range GlobalHub.broadcast {
		GlobalHub.mu.RLock()
		for _, client := range GlobalHub.clients {
			udpConn.WriteToUDP(message, client.Addr)
			client.LastSeen = time.Now()
		}
		GlobalHub.mu.RUnlock()
	}
}
