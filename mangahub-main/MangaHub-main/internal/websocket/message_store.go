package websocket

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// MessageStore is the durable boundary; a write must finish before broadcast.
type MessageStore interface {
	Save(context.Context, Message) error
	Recent(context.Context, string, int) ([]Message, error)
}

type SQLiteMessageStore struct{ db *sql.DB }

func NewSQLiteMessageStore(db *sql.DB) *SQLiteMessageStore {
	return &SQLiteMessageStore{db: db}
}

func (s *SQLiteMessageStore) Save(ctx context.Context, message Message) error {
	result, err := s.db.ExecContext(ctx, `INSERT INTO chat_messages (id,user_id,username,room,text,created_at)
		SELECT ?,?,?,?,?,? WHERE EXISTS (SELECT 1 FROM users WHERE id=? AND username=?)`,
		message.ID, message.UserID, message.Username, message.Room, message.Text, message.CreatedAt, message.UserID, message.Username)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("chat author does not exist")
	}
	return nil
}

func (s *SQLiteMessageStore) Recent(ctx context.Context, room string, limit int) ([]Message, error) {
	if limit < 1 || limit > 50 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,username,room,text,created_at FROM
		(SELECT sequence,id,username,room,text,created_at FROM chat_messages WHERE room=? ORDER BY sequence DESC LIMIT ?)
		ORDER BY sequence ASC`, room, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	messages := make([]Message, 0)
	for rows.Next() {
		message := Message{Type: "chat"}
		if err := rows.Scan(&message.ID, &message.Username, &message.Room, &message.Text, &message.CreatedAt); err != nil {
			return nil, err
		}
		created, err := time.Parse(time.RFC3339Nano, message.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("invalid stored chat timestamp")
		}
		message.Time = created.Local().Format("15:04")
		messages = append(messages, message)
	}
	return messages, rows.Err()
}

// NewPersistentHub is used by the canonical backend. NewHub remains available
// for isolated protocol tests and the older standalone demonstration program.
func NewPersistentHub(store MessageStore) *Hub {
	hub := NewHub()
	hub.store = store
	return hub
}

// sendToClient shares the membership lock with channel closure in Hub.Run.
func (h *Hub) sendToClient(client *Client, frame []byte) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if !h.clients[client] {
		return false
	}
	select {
	case client.Send <- frame:
		return true
	default:
		return false
	}
}
