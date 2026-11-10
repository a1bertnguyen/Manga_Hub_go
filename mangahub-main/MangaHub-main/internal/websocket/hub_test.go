package websocket

import (
	"encoding/json"
	"testing"
	"time"
)

func TestWebSocketHub_RoomIsolation(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	clientA1 := &Client{
		Hub:      hub,
		Send:     make(chan []byte, 10),
		Username: "alice",
		Room:     "shounen",
	}
	clientA2 := &Client{
		Hub:      hub,
		Send:     make(chan []byte, 10),
		Username: "bob",
		Room:     "shounen",
	}
	clientB := &Client{
		Hub:      hub,
		Send:     make(chan []byte, 10),
		Username: "charlie",
		Room:     "seinen",
	}

	hub.Register <- clientA1
	hub.Register <- clientA2
	hub.Register <- clientB
	time.Sleep(50 * time.Millisecond)

	if count := hub.GetClientCount(); count != 3 {
		t.Errorf("Expected 3 connected clients, got %d", count)
	}

	// 1. Send chat message to room "shounen"
	shounenMsg := Message{
		Type:     "chat",
		Username: "alice",
		Text:     "Luffy gears up!",
		Room:     "shounen",
	}
	data, _ := json.Marshal(shounenMsg)
	hub.Broadcast <- data

	// Verify clientA1 received it
	select {
	case msg := <-clientA1.Send:
		var parsed Message
		_ = json.Unmarshal(msg, &parsed)
		if parsed.Text != "Luffy gears up!" || parsed.Room != "shounen" {
			t.Errorf("Unexpected message in clientA1: %+v", parsed)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Timeout: clientA1 did not receive shounen message")
	}

	// Verify clientA2 received it
	select {
	case msg := <-clientA2.Send:
		var parsed Message
		_ = json.Unmarshal(msg, &parsed)
		if parsed.Text != "Luffy gears up!" {
			t.Errorf("Unexpected message in clientA2: %+v", parsed)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Timeout: clientA2 did not receive shounen message")
	}

	// CRITICAL ROOM ISOLATION CHECK: clientB in "seinen" must NOT receive "shounen" message
	select {
	case msg := <-clientB.Send:
		t.Fatalf("ROOM ISOLATION VIOLATION: Seinen client received Shounen message: %s", string(msg))
	case <-time.After(150 * time.Millisecond):
		// Expected: nothing received
	}

	// 2. Send chat message to room "seinen"
	seinenMsg := Message{
		Type:     "chat",
		Username: "charlie",
		Text:     "Berserk discussion",
		Room:     "seinen",
	}
	dataSeinen, _ := json.Marshal(seinenMsg)
	hub.Broadcast <- dataSeinen

	// Verify clientB received it
	select {
	case msg := <-clientB.Send:
		var parsed Message
		_ = json.Unmarshal(msg, &parsed)
		if parsed.Text != "Berserk discussion" || parsed.Room != "seinen" {
			t.Errorf("Unexpected message in clientB: %+v", parsed)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Timeout: clientB did not receive seinen message")
	}

	// Verify clientA1 does not receive seinen message
	select {
	case msg := <-clientA1.Send:
		t.Fatalf("ROOM ISOLATION VIOLATION: Shounen client received Seinen message: %s", string(msg))
	case <-time.After(150 * time.Millisecond):
		// Expected: nothing received
	}
}

func TestWebSocketHub_HistoryAndDisconnect(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	client := &Client{
		Hub:      hub,
		Send:     make(chan []byte, 10),
		Username: "tester",
		Room:     "general",
	}

	hub.Register <- client
	time.Sleep(30 * time.Millisecond)

	msg := Message{
		Type:     "chat",
		Username: "tester",
		Text:     "Hello MangaHub",
		Room:     "general",
	}
	data, _ := json.Marshal(msg)
	hub.Broadcast <- data

	time.Sleep(50 * time.Millisecond)
	history := hub.GetMessageHistory("general")
	if len(history) != 1 || history[0].Text != "Hello MangaHub" {
		t.Fatalf("Expected 1 history item 'Hello MangaHub', got: %+v", history)
	}

	// Unregister client
	hub.Unregister <- client
	time.Sleep(50 * time.Millisecond)

	if count := hub.GetClientCount(); count != 0 {
		t.Errorf("Expected 0 clients after unregister, got %d", count)
	}
}
