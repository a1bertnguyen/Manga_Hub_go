package udp

import (
	"encoding/json"
	"log"
	"net"
	"strings"
	"time"
)

var udpConn *net.UDPConn // Shared UDP connection used for sending and receiving packets

func StartUDPListener(addr string) {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		log.Fatalf("UDP resolve error: %v", err)
	}

	udpConn, err = net.ListenUDP("udp", udpAddr)
	if err != nil {
		log.Fatalf("UDP listen error: %v", err)
	}
	log.Printf("[UDP Listener] Server listening on %s", addr)

	go readPump()
}

func readPump() {
	buffer := make([]byte, 4096)
	for {
		n, clientAddr, err := udpConn.ReadFromUDP(buffer)
		if err != nil {
			if strings.Contains(err.Error(), "use of closed network connection") {
				return
			}
			log.Printf("[UDP Listener] Read error: %v", err)
			continue
		}

		raw := strings.TrimSpace(string(buffer[:n]))
		addrStr := clientAddr.String()

		var req struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal([]byte(raw), &req)

		isSubscribe := req.Type == "subscribe" || raw == "PING" || strings.ToUpper(raw) == "SUBSCRIBE"
		isUnsubscribe := req.Type == "unsubscribe" || strings.ToUpper(raw) == "UNSUBSCRIBE"

		if isUnsubscribe {
			GlobalHub.Unregister <- &ClientAddr{Addr: clientAddr}
			resp := []byte("{\"type\":\"unsubscribed\",\"status\":\"ok\"}\n")
			_, _ = udpConn.WriteToUDP(resp, clientAddr)
			log.Printf("[UDP Listener] Unsubscribed: %s", addrStr)
			continue
		}

		if isSubscribe {
			client := &ClientAddr{
				Addr:     clientAddr,
				LastSeen: time.Now(),
			}
			GlobalHub.Register <- client
			resp := []byte("{\"type\":\"subscribed\",\"status\":\"ok\"}\n")
			_, _ = udpConn.WriteToUDP(resp, clientAddr)
			log.Printf("[UDP Listener] Subscribed: %s", addrStr)
			continue
		}
	}
}


