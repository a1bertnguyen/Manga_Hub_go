package udp

import (
	"encoding/json"
	"net"
	"testing"
	"time"
)

func TestUDPHub_RegisterAndUnregister(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	addr1 := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 54321}
	client1 := &ClientAddr{Addr: addr1, LastSeen: time.Now()}

	hub.Register <- client1
	time.Sleep(30 * time.Millisecond)

	if count := hub.GetClientCount(); count != 1 {
		t.Errorf("Expected 1 client, got %d", count)
	}

	hub.Unregister <- client1
	time.Sleep(30 * time.Millisecond)

	if count := hub.GetClientCount(); count != 0 {
		t.Errorf("Expected 0 clients after unregister, got %d", count)
	}
}

func TestUDPHub_BroadcastChapterReleasePayload(t *testing.T) {
	hub := NewHub()

	// Broadcast without running loop should put item into hub.broadcast channel
	hub.BroadcastChapterRelease("one-piece", "One Piece", 1105)

	select {
	case data := <-hub.broadcast:
		var payload map[string]interface{}
		if err := json.Unmarshal(data, &payload); err != nil {
			t.Fatalf("Failed to unmarshal broadcast payload: %v", err)
		}
		if payload["type"] != "new_chapter" {
			t.Errorf("Expected type 'new_chapter', got %v", payload["type"])
		}
		if payload["manga_id"] != "one-piece" {
			t.Errorf("Expected manga_id 'one-piece', got %v", payload["manga_id"])
		}
		if payload["chapter"] != float64(1105) {
			t.Errorf("Expected chapter 1105, got %v", payload["chapter"])
		}
		if payload["manga_title"] != "One Piece" {
			t.Errorf("Expected manga_title 'One Piece', got %v", payload["manga_title"])
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Timeout waiting for broadcast payload")
	}
}

func TestUDPNetwork_SubscribeAndReceive(t *testing.T) {
	// Start listener on a random local UDP port
	serverConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("Failed to start test UDP listener: %v", err)
	}
	defer serverConn.Close()

	serverAddr := serverConn.LocalAddr().(*net.UDPAddr)
	udpConn = serverConn // wire into package global
	go GlobalHub.Run()
	go readPump()

	// Create client socket
	clientConn, err := net.DialUDP("udp", nil, serverAddr)
	if err != nil {
		t.Fatalf("Failed to dial UDP server: %v", err)
	}
	defer clientConn.Close()

	_ = clientConn.SetDeadline(time.Now().Add(2 * time.Second))

	// 1. Send SUBSCRIBE datagram
	subMsg := []byte("{\"type\":\"subscribe\"}\n")
	if _, err := clientConn.Write(subMsg); err != nil {
		t.Fatalf("Failed to send subscribe datagram: %v", err)
	}

	buf := make([]byte, 2048)
	n, err := clientConn.Read(buf)
	if err != nil {
		t.Fatalf("Failed to read subscribe response: %v", err)
	}
	if !json.Valid(buf[:n]) {
		t.Fatalf("Invalid JSON in subscribe response: %s", string(buf[:n]))
	}

	// 2. Hub broadcasts new chapter
	GlobalHub.BroadcastChapterRelease("jujutsu-kaisen", "Jujutsu Kaisen", 270)

	n, err = clientConn.Read(buf)
	if err != nil {
		t.Fatalf("Failed to read chapter release broadcast: %v", err)
	}

	var alert struct {
		Type    string `json:"type"`
		MangaID string `json:"manga_id"`
		Chapter int    `json:"chapter"`
	}
	if err := json.Unmarshal(buf[:n], &alert); err != nil {
		t.Fatalf("Failed to unmarshal alert datagram: %v", err)
	}
	if alert.Type != "new_chapter" || alert.MangaID != "jujutsu-kaisen" || alert.Chapter != 270 {
		t.Errorf("Unexpected alert datagram: %+v", alert)
	}

	// 3. Send UNSUBSCRIBE datagram
	unsubMsg := []byte("{\"type\":\"unsubscribe\"}\n")
	if _, err := clientConn.Write(unsubMsg); err != nil {
		t.Fatalf("Failed to send unsubscribe datagram: %v", err)
	}

	n, err = clientConn.Read(buf)
	if err != nil {
		t.Fatalf("Failed to read unsubscribe response: %v", err)
	}
	var unsubResp struct {
		Type   string `json:"type"`
		Status string `json:"status"`
	}
	_ = json.Unmarshal(buf[:n], &unsubResp)
	if unsubResp.Type != "unsubscribed" {
		t.Errorf("Expected unsubscribed response, got: %s", string(buf[:n]))
	}
}
