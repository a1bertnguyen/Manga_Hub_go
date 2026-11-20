package tui

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAPIProgressUsesTCPSession(t *testing.T) {
	var header, authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header = r.Header.Get("X-Session-ID")
		authorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	state := &AppState{BaseURL: server.URL, Token: "jwt", UserID: "alice", TCPSessionID: "server-issued", TCPConnected: true}
	api := NewAPIClient(state)
	if err := api.UpdateProgress("manga", 3, "reading"); err != nil {
		t.Fatal(err)
	}
	if header != "server-issued" || authorization != "Bearer jwt" {
		t.Fatal("progress request omitted authenticated source session")
	}
	if err := api.AddToLibrary("manga", "reading"); err != nil {
		t.Fatal(err)
	}
	if header != "" {
		t.Fatal("unrelated request included TCP session")
	}
	state.setConnection("tcp", false)
	if state.tcpSessionID() != "" {
		t.Fatal("disconnect did not clear TCP session")
	}
	if err := api.UpdateProgress("manga", 4, "reading"); err != nil {
		t.Fatal(err)
	}
	if header != "" {
		t.Fatal("disconnected client reused a stale TCP session")
	}
	state.TCPSessionID = "previous-login"
	state.setSession("next-token", "bob", "Bob")
	if state.tcpSessionID() != "" || state.connected("tcp") {
		t.Fatal("account change reused the previous TCP session")
	}
}
