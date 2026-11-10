package tui

import (
	"os"
	"testing"

	"github.com/rivo/tview"
)

func TestSession_SaveLoadClear(t *testing.T) {
	// Clean up any test session
	_ = ClearSession()
	defer ClearSession()

	_, err := LoadSession()
	if err == nil {
		t.Fatalf("expected error loading non-existent session, got nil")
	}

	testToken := "test_token_xyz_123"
	testUserID := "usr_test123"
	testUsername := "tester"

	if err := SaveSession(testToken, testUserID, testUsername); err != nil {
		t.Fatalf("SaveSession failed: %v", err)
	}

	sess, err := LoadSession()
	if err != nil {
		t.Fatalf("LoadSession failed: %v", err)
	}

	if sess.Token != testToken || sess.UserID != testUserID || sess.Username != testUsername {
		t.Fatalf("session mismatch: got %+v", sess)
	}

	if err := ClearSession(); err != nil {
		t.Fatalf("ClearSession failed: %v", err)
	}

	if _, err := os.Stat(sessionFileName); !os.IsNotExist(err) {
		t.Fatalf("session file still exists after ClearSession")
	}
}

func TestUIController_SetupLayout(t *testing.T) {
	app := tview.NewApplication()
	pages := tview.NewPages()

	state := NewAppState(app, pages, "http://localhost:8080", "localhost:9090", "localhost:9091", "ws://localhost:8080/ws")
	api := NewAPIClient(state)
	tcp := NewTCPWorker(state)
	udp := NewUDPWorker(state)
	chat := NewChatClient(state)

	controller := NewUIController(state, api, tcp, udp, chat)
	layout := controller.SetupLayout()

	if layout == nil {
		t.Fatalf("SetupLayout returned nil")
	}

	// Verify tab pages are registered
	expectedPages := []string{"browser", "library", "discussion", "notifications", "help", "auth"}
	for _, pageName := range expectedPages {
		if !pages.HasPage(pageName) {
			t.Errorf("missing page in layout: %s", pageName)
		}
	}
}
