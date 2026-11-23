package tui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/gorilla/websocket"
	"github.com/rivo/tview"
	"mangahub/internal/database"
	wsHub "mangahub/internal/websocket"
)

func TestDiscussionReplaysSQLiteHistoryAndRejectsFailedSave(t *testing.T) {
	if err := database.Initialize(filepath.Join(t.TempDir(), "messages.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.DB.Exec(`INSERT INTO users(id,username,email,password_hash) VALUES('a','alice','a@example.test','fixture')`); err != nil {
		t.Fatal(err)
	}
	store := wsHub.NewSQLiteMessageStore(database.DB)
	for _, message := range []wsHub.Message{
		{ID: "saved-general", UserID: "a", Username: "alice", Room: "general", Text: "persisted-general-marker [red]", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
		{ID: "saved-other", UserID: "a", Username: "alice", Room: "spoilers", Text: "isolated-history-marker", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
	} {
		if err := store.Save(context.Background(), message); err != nil {
			t.Fatal(err)
		}
	}
	hub := wsHub.NewPersistentHub(store)
	go hub.Run()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		client := &wsHub.Client{Hub: hub, Conn: conn, Send: make(chan []byte, 256), Username: "alice", UserID: "a", Room: r.URL.Query().Get("room")}
		hub.Register <- client
		go client.WritePump()
		client.ReadPump()
	}))
	t.Cleanup(server.Close)
	c, screen := newTestUI(t, nil)
	t.Cleanup(c.chat.Close)
	uiDo(t, c, func() {
		c.state.setSession("alice", "a", "alice")
		c.state.WSURL = "ws" + strings.TrimPrefix(server.URL, "http")
		c.SwitchTab("discussion")
	})
	acceptanceWait(t, c, "stored history rendered", func() bool { return strings.Contains(discussionScreenText(screen), "persisted-general-marker") })
	uiDo(t, c, func() {
		if strings.Contains(c.state.ChatMessagesView.GetText(true), "isolated-history-marker") {
			t.Error("another room's stored history leaked")
		}
	})
	if _, err := database.DB.Exec("PRAGMA query_only=ON"); err != nil {
		t.Fatal(err)
	}
	for _, char := range "failed-save-draft" {
		screen.InjectKey(tcell.KeyRune, char, tcell.ModNone)
	}
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	acceptanceWait(t, c, "database failure feedback", func() bool { return strings.Contains(c.state.StatusBar.GetText(true), "could not be saved") })
	uiDo(t, c, func() {
		if c.focus["discussion"][0].(*tview.InputField).GetText() != "failed-save-draft" || strings.Contains(c.state.ChatMessagesView.GetText(true), "failed-save-draft") {
			t.Error("failed save lost draft or generated a false successful echo")
		}
	})
	var count int
	_ = database.DB.QueryRow("SELECT COUNT(*) FROM chat_messages").Scan(&count)
	if count != 2 {
		t.Fatal("failed save mutated SQLite")
	}
}

// Invoked after a real backend restart by scripts/verify_database_roundtrip.py.
func TestLiveDiscussionHistoryTUI(t *testing.T) {
	marker := os.Getenv("MANGAHUB_HISTORY_MARKER")
	if marker == "" {
		t.Skip("run python scripts/verify_database_roundtrip.py")
	}
	c, screen := newTestUI(t, nil)
	t.Cleanup(c.chat.Close)
	uiDo(t, c, func() {
		c.state.setSession(os.Getenv("MANGAHUB_HISTORY_TOKEN"), os.Getenv("MANGAHUB_HISTORY_USER_ID"), os.Getenv("MANGAHUB_HISTORY_USERNAME"))
		c.state.WSURL = os.Getenv("MANGAHUB_HISTORY_WS")
		c.SwitchTab("discussion")
	})
	acceptanceWait(t, c, "persisted history after backend restart", func() bool { return strings.Contains(discussionScreenText(screen), marker) })
	uiDo(t, c, func() {
		if strings.Count(c.state.ChatMessagesView.GetText(true), marker) != 1 {
			t.Error("history replay duplicated a stored message")
		}
	})
}

func TestReplayLiveOverlapIsAcknowledgedOnce(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		var message map[string]string
		if conn.ReadJSON(&message) != nil {
			return
		}
		message["username"], message["room"] = "alice", "general"
		_ = conn.WriteJSON(map[string]interface{}{"type": "history", "room": "general", "messages": []map[string]string{message}})
		_ = conn.WriteJSON(message)
		_, _, _ = conn.ReadMessage()
	}))
	defer server.Close()
	c, _ := newTestUI(t, nil)
	t.Cleanup(c.chat.Close)
	uiDo(t, c, func() {
		c.state.setSession("alice", "a", "alice")
		c.state.WSURL = "ws" + strings.TrimPrefix(server.URL, "http")
		c.SwitchTab("discussion")
	})
	acceptanceWait(t, c, "connected", func() bool { return c.state.connected("ws") })
	if err := c.chat.SendMessage("overlap-marker"); err != nil {
		t.Fatal(err)
	}
	uiDo(t, c, func() {
		if strings.Count(c.state.ChatMessagesView.GetText(true), "overlap-marker") != 1 {
			t.Error("live echo duplicated replay content")
		}
	})
}
