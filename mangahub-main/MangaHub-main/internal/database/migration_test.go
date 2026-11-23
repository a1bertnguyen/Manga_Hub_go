package database

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestLegacyDatabaseGetsChatAndVersionWithoutLosingRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	old, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = old.Exec(`CREATE TABLE user_progress(user_id TEXT,manga_id TEXT,current_chapter INTEGER,status TEXT,updated_at TIMESTAMP,PRIMARY KEY(user_id,manga_id)); INSERT INTO user_progress VALUES('old-user','old-manga',7,'reading',CURRENT_TIMESTAMP)`)
	_ = old.Close()
	if err != nil {
		t.Fatal(err)
	}
	if err := Initialize(path); err != nil {
		t.Fatal(err)
	}
	defer Close()
	var chapter, version, chatCount int
	if err := DB.QueryRow("SELECT current_chapter,version FROM user_progress").Scan(&chapter, &version); err != nil {
		t.Fatal(err)
	}
	if err := DB.QueryRow("SELECT COUNT(*) FROM chat_messages").Scan(&chatCount); err != nil {
		t.Fatal(err)
	}
	if chapter != 7 || version != 1 || chatCount != 0 {
		t.Fatal("migration changed legacy data or chat schema")
	}
}
