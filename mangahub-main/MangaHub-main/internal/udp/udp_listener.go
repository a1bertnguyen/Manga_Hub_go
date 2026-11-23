package udp

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"strings"
	"sync/atomic"
	"time"
)

var udpConn atomic.Pointer[net.UDPConn]

func IsListening() bool { return udpConn.Load() != nil }

func StartUDPListener(addr string) error {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return fmt.Errorf("resolve UDP address: %w", err)
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return fmt.Errorf("listen UDP: %w", err)
	}
	udpConn.Store(conn)
	log.Printf("[UDP Listener] Server listening on %s", addr)

	go readPump(conn)
	return nil
}

func readPump(conn *net.UDPConn) {
	defer udpConn.CompareAndSwap(conn, nil)
	buffer := make([]byte, 4096)
	for {
		n, clientAddr, err := conn.ReadFromUDP(buffer)
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
			_, _ = conn.WriteToUDP(resp, clientAddr)
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
			_, _ = conn.WriteToUDP(resp, clientAddr)
			log.Printf("[UDP Listener] Subscribed: %s", addrStr)
			continue
		}
	}
}
