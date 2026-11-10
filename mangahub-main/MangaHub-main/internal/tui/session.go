package tui

import (
	"encoding/json"
	"os"
)

// Session represents saved authentication credentials for persistent TUI logins
type Session struct {
	Token    string `json:"token"`
	UserID   string `json:"user_id"`
	Username string `json:"username"`
}

const sessionFileName = ".tui_session.json"

// SaveSession writes credentials to a local session file
func SaveSession(token, userID, username string) error {
	s := Session{
		Token:    token,
		UserID:   userID,
		Username: username,
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(sessionFileName, data, 0600)
}

// LoadSession reads credentials from the local session file if present
func LoadSession() (*Session, error) {
	data, err := os.ReadFile(sessionFileName)
	if err != nil {
		return nil, err
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	if s.Token == "" || s.Username == "" {
		return nil, os.ErrNotExist
	}
	return &s, nil
}

// ClearSession removes the local session file on logout
func ClearSession() error {
	if _, err := os.Stat(sessionFileName); os.IsNotExist(err) {
		return nil
	}
	return os.Remove(sessionFileName)
}
