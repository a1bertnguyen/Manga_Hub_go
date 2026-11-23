package websocket

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"mangahub/internal/database"
)

func TestMessageStorePersistsRoomHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chat.db")
	if err := database.Initialize(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.DB.Exec(`INSERT INTO users(id,username,email,password_hash) VALUES ('alice-id','alice','a@example.test','fixture')`); err != nil {
		t.Fatal(err)
	}
	store := NewSQLiteMessageStore(database.DB)
	for i := 0; i < 60; i++ {
		message := Message{ID: fmt.Sprintf("id-%02d", i), UserID: "alice-id", Username: "alice", Room: "general", Text: fmt.Sprintf("message-%02d", i), CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
		if err := store.Save(context.Background(), message); err != nil {
			t.Fatal(err)
		}
	}
	message := Message{ID: "private-room", UserID: "alice-id", Username: "alice", Room: "spoilers", Text: "Other room", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if err := store.Save(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	if err := database.Initialize(path); err != nil {
		t.Fatal(err)
	}
	store = NewSQLiteMessageStore(database.DB)
	history, err := store.Recent(context.Background(), "general", 50)
	if err != nil || len(history) != 50 || history[0].ID != "id-10" || history[49].ID != "id-59" {
		t.Fatalf("history limit/order did not survive reopen: %v", err)
	}
	for _, message := range history {
		if message.Room != "general" || message.Type != "chat" || message.Time == "" {
			t.Fatal("room/type/timestamp mismatch")
		}
	}
	var count int
	_ = database.DB.QueryRow("SELECT COUNT(*) FROM chat_messages").Scan(&count)
	if count != 61 {
		t.Fatal("replay limit deleted stored messages")
	}
	message.ID = "invalid-author"
	message.UserID = "unknown"
	if err := store.Save(context.Background(), message); err == nil {
		t.Fatal("unknown author was stored")
	}
	if _, err := database.DB.Exec("PRAGMA query_only=ON"); err != nil {
		t.Fatal(err)
	}
	message.ID = "failed-write"
	message.UserID = "alice-id"
	if err := store.Save(context.Background(), message); err == nil {
		t.Fatal("read-only database reported successful write")
	}
}
