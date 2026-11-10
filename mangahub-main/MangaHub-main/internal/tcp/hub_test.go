package tcp

import (
	"encoding/json"
	"net"
	"testing"
	"time"

	"mangahub/pkg/models"
)

type dummyConn struct {
	net.Conn
}

func (d *dummyConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345}
}
func (d *dummyConn) Close() error { return nil }

func TestHub_UserIsolation(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	clientA1 := &Client{
		Conn:   &dummyConn{},
		UserID: "user_alice",
		Send:   make(chan []byte, 10),
	}
	clientA2 := &Client{
		Conn:   &dummyConn{},
		UserID: "user_alice",
		Send:   make(chan []byte, 10),
	}
	clientB := &Client{
		Conn:   &dummyConn{},
		UserID: "user_bob",
		Send:   make(chan []byte, 10),
	}

	hub.Register <- clientA1
	hub.Register <- clientA2
	hub.Register <- clientB

	// Allow registration loop to process
	time.Sleep(50 * time.Millisecond)

	// Broadcast progress update for user_alice
	update := models.UserProgress{
		UserID:         "user_alice",
		MangaID:        "one-piece",
		CurrentChapter: 105,
		Status:         "reading",
		Version:        3,
	}
	hub.BroadcastProgress(update, "alice", "One Piece")

	// Verify clientA1 received the update
	select {
	case msg := <-clientA1.Send:
		var parsed map[string]interface{}
		if err := json.Unmarshal(msg, &parsed); err != nil {
			t.Fatalf("Failed to unmarshal clientA1 message: %v", err)
		}
		if parsed["user_id"] != "user_alice" || parsed["chapter"] != float64(105) || parsed["version"] != float64(3) {
			t.Errorf("Unexpected payload in clientA1: %+v", parsed)
		}
		if parsed["type"] != "progress_updated" {
			t.Errorf("Expected type 'progress_updated', got %v", parsed["type"])
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Timeout: clientA1 did not receive progress update")
	}

	// Verify clientA2 received the update
	select {
	case msg := <-clientA2.Send:
		var parsed map[string]interface{}
		if err := json.Unmarshal(msg, &parsed); err != nil {
			t.Fatalf("Failed to unmarshal clientA2 message: %v", err)
		}
		if parsed["user_id"] != "user_alice" {
			t.Errorf("Unexpected user in clientA2: %v", parsed["user_id"])
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Timeout: clientA2 did not receive progress update")
	}

	// CRITICAL USER ISOLATION CHECK: clientB (user_bob) must NOT receive user_alice's update
	select {
	case msg := <-clientB.Send:
		t.Fatalf("CRITICAL SECURITY VIOLATION: User Bob received Alice's private progress update: %s", string(msg))
	case <-time.After(150 * time.Millisecond):
		// Expected: Bob received nothing
	}
}

func TestHub_Unregister(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	client := &Client{
		Conn:   &dummyConn{},
		UserID: "user_disconnect_test",
		Send:   make(chan []byte, 5),
	}

	hub.Register <- client
	time.Sleep(30 * time.Millisecond)

	hub.Unregister <- client
	time.Sleep(30 * time.Millisecond)

	// Channel should be closed
	_, ok := <-client.Send
	if ok {
		t.Errorf("Expected client.Send channel to be closed after unregistering")
	}
}
