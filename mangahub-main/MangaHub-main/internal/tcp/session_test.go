package tcp

import (
	"testing"
	"time"

	"mangahub/pkg/models"
)

func TestHubExcludesOnlySourceSession(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	clients := make([]*Client, 3)
	for i, user := range []string{"alice", "alice", "bob"} {
		clients[i] = &Client{Conn: &dummyConn{}, UserID: user, SessionID: []string{"a", "b", "c"}[i], Send: make(chan []byte, 8), Registered: make(chan struct{})}
		hub.Register <- clients[i]
		<-clients[i].Registered
	}
	if !hub.OwnsSession("alice", "a") || hub.OwnsSession("bob", "a") || hub.OwnsSession("alice", "") || hub.OwnsSession("alice", "missing") {
		t.Fatal("session ownership does not match authenticated membership")
	}
	for _, source := range []int{0, 1} {
		hub.BroadcastProgress(models.UserProgress{UserID: "alice", CurrentChapter: source + 1}, "Alice", "Manga", clients[source].SessionID)
		select {
		case <-clients[1-source].Send:
		case <-time.After(time.Second):
			t.Fatal("other same-user session received nothing")
		}
		for _, excluded := range []int{source, 2} {
			select {
			case <-clients[excluded].Send:
				t.Fatalf("source or different user received an event: client %d", excluded)
			case <-time.After(30 * time.Millisecond):
			}
		}
	}
	hub.Unregister <- clients[0]
	<-clients[0].Send // unregister closes the channel after removing membership
	if hub.OwnsSession("alice", "a") {
		t.Fatal("disconnected session remained valid")
	}
}
