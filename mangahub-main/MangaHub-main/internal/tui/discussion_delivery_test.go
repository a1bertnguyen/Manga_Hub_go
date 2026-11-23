package tui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	wsHub "mangahub/internal/websocket"
	"mangahub/pkg/models"

	"github.com/gdamore/tcell/v2"
	"github.com/gorilla/websocket"
	"github.com/rivo/tview"
)

func discussionScreenText(screen tcell.SimulationScreen) string {
	cells, width, _ := screen.GetContents()
	var text strings.Builder
	for i, cell := range cells {
		if len(cell.Runes) == 0 {
			text.WriteRune(' ')
		} else {
			text.WriteRune(cell.Runes[0])
		}
		if (i+1)%width == 0 {
			text.WriteRune('\n')
		}
	}
	return text.String()
}

func newDiscussionPair(t *testing.T) (*UIController, tcell.SimulationScreen, *UIController, tcell.SimulationScreen) {
	t.Helper()
	hub := wsHub.NewHub()
	go hub.Run()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		client := &wsHub.Client{Hub: hub, Conn: conn, Send: make(chan []byte, 256), Username: strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), Room: r.URL.Query().Get("room")}
		hub.Register <- client
		go client.WritePump()
		client.ReadPump()
	}))
	t.Cleanup(server.Close)
	makeClient := func(name string) (*UIController, tcell.SimulationScreen) {
		c, screen := newTestUI(t, nil)
		t.Cleanup(c.chat.Close)
		uiDo(t, c, func() {
			c.state.setSession(name, name, name)
			c.state.WSURL = "ws" + strings.TrimPrefix(server.URL, "http")
			c.SwitchTab("discussion")
		})
		acceptanceWait(t, c, "WebSocket discussion connection", func() bool { return c.state.connected("ws") })
		return c, screen
	}
	a, sa := makeClient("alice")
	b, sb := makeClient("bob")
	return a, sa, b, sb
}

func TestDiscussionMessageVisibleOnBothScreens(t *testing.T) {
	for _, size := range [][2]int{{120, 38}, {80, 24}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			a, screenA, b, screenB := newDiscussionPair(t)
			uiDo(t, a, func() { screenA.SetSize(size[0], size[1]) })
			uiDo(t, b, func() { screenB.SetSize(size[0], size[1]) })
			marker := "hello-discussion-123"
			for _, char := range marker {
				screenA.InjectKey(tcell.KeyRune, char, tcell.ModNone)
			}
			screenA.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
			for _, client := range []struct {
				c *UIController
				s tcell.SimulationScreen
			}{{a, screenA}, {b, screenB}} {
				acceptanceWait(t, client.c, "actual rendered message", func() bool { return strings.Contains(discussionScreenText(client.s), marker) })
			}
		})
	}
}

func TestDiscussionDisplaysLiteralMarkup(t *testing.T) {
	a, screenA, b, screenB := newDiscussionPair(t)
	message := "literal [red] [white:white] visible-marker"
	if err := a.chat.SendMessage(message); err != nil {
		t.Fatal(err)
	}
	acceptanceWait(t, b, "literal message delivered", func() bool { return strings.Contains(b.state.ChatMessagesView.GetText(false), "visible-marker") })
	for _, client := range []struct {
		c *UIController
		s tcell.SimulationScreen
	}{{a, screenA}, {b, screenB}} {
		uiDo(t, client.c, func() {
			if !strings.Contains(client.c.state.ChatMessagesView.GetText(true), message) {
				t.Error("user text was interpreted as TUI formatting instead of displayed literally")
			}
		})
	}
}

func TestDiscussionUnicodeMessageDoesNotDisappear(t *testing.T) {
	a, _, b, _ := newDiscussionPair(t)
	message := strings.Repeat("Xin chào bạn! ", 50) + "unicode-delivery-marker"
	if err := a.chat.SendMessage(message); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		arrived := false
		uiDo(t, b, func() { arrived = strings.Contains(b.state.ChatMessagesView.GetText(true), "unicode-delivery-marker") })
		if arrived {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("SendMessage returned success but server closed the connection without delivering the Vietnamese message")
}

func TestDiscussionKeepsInputUntilServerEcho(t *testing.T) {
	received := make(chan map[string]string, 1)
	allowEcho := make(chan struct{})
	releaseEcho := sync.OnceFunc(func() { close(allowEcho) })
	defer releaseEcho()
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
		received <- message
		<-allowEcho
		message["username"], message["room"] = "alice", "general"
		_ = conn.WriteJSON(message)
		_, _, _ = conn.ReadMessage()
	}))
	defer server.Close()
	c, screen := newTestUI(t, nil)
	t.Cleanup(c.chat.Close)
	uiDo(t, c, func() {
		c.state.setSession("alice", "alice", "alice")
		c.state.WSURL = "ws" + strings.TrimPrefix(server.URL, "http")
		c.SwitchTab("discussion")
	})
	acceptanceWait(t, c, "chat connected", func() bool { return c.state.connected("ws") })
	for _, char := range " keep-until-confirmed " {
		screen.InjectKey(tcell.KeyRune, char, tcell.ModNone)
	}
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	select {
	case message := <-received:
		if message["id"] == "" || message["text"] != "keep-until-confirmed" {
			t.Fatal("message has no correlation ID")
		}
	case <-time.After(time.Second):
		t.Fatal("message never reached server")
	}
	uiDo(t, c, func() {
		if c.focus["discussion"][0].(*tview.InputField).GetText() != " keep-until-confirmed " {
			t.Error("input cleared before server echoed the message")
		}
	})
	releaseEcho()
	acceptanceWait(t, c, "input cleared only after confirmed echo", func() bool {
		return c.focus["discussion"][0].(*tview.InputField).GetText() == "" && strings.Contains(discussionScreenText(screen), "keep-until-confirmed")
	})
}

func TestDiscussionOversizeRetainsTextAndConnection(t *testing.T) {
	a, _, b, _ := newDiscussionPair(t)
	message := strings.Repeat("x", models.MaxChatFrameBytes+1)
	uiDo(t, a, func() {
		input := a.focus["discussion"][0].(*tview.InputField)
		input.SetText(message)
		a.state.App.SetFocus(input)
		uiKey(a, tcell.KeyEnter, 0)
	})
	acceptanceWait(t, a, "oversize feedback", func() bool { return strings.Contains(a.state.StatusBar.GetText(true), "byte") })
	uiDo(t, a, func() {
		if a.focus["discussion"][0].(*tview.InputField).GetText() != message || !a.state.connected("ws") {
			t.Error("oversize message lost input or broke the connection")
		}
	})
	if err := a.chat.SendMessage("after-oversize-marker"); err != nil {
		t.Fatal(err)
	}
	acceptanceWait(t, b, "valid follow-up", func() bool { return strings.Contains(b.state.ChatMessagesView.GetText(true), "after-oversize-marker") })
}

func TestDiscussionDisconnectReportsFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		var message map[string]string
		_ = conn.ReadJSON(&message) // Simulate a backend dropping the frame before echo.
	}))
	defer server.Close()
	c, screen := newTestUI(t, nil)
	t.Cleanup(c.chat.Close)
	uiDo(t, c, func() {
		c.state.setSession("alice", "alice", "alice")
		c.state.WSURL = "ws" + strings.TrimPrefix(server.URL, "http")
		c.SwitchTab("discussion")
	})
	acceptanceWait(t, c, "connected", func() bool { return c.state.connected("ws") })
	for _, char := range "not-confirmed" {
		screen.InjectKey(tcell.KeyRune, char, tcell.ModNone)
	}
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	acceptanceWait(t, c, "failure feedback with retained text", func() bool {
		return !c.state.connected("ws") && c.focus["discussion"][0].(*tview.InputField).GetText() == "not-confirmed" && strings.Contains(c.state.StatusBar.GetText(true), "Chat send error")
	})
}

// Ensure the protocol's optional ID remains compatible with existing clients.
func TestDiscussionLegacyFrameStillDecodes(t *testing.T) {
	var message wsHub.Message
	if err := json.Unmarshal([]byte(`{"type":"chat","text":"legacy","room":"general"}`), &message); err != nil || message.ID != "" || message.Text != "legacy" {
		t.Fatal("legacy chat frame stopped decoding")
	}
}

func TestDiscussionNoEchoRetainsInputAndStaysInteractive(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return // Keep the connection open but deliberately never acknowledge.
			}
		}
	}))
	defer server.Close()
	c, screen := newTestUI(t, nil)
	t.Cleanup(c.chat.Close)
	uiDo(t, c, func() {
		c.state.setSession("alice", "alice", "alice")
		c.state.WSURL = "ws" + strings.TrimPrefix(server.URL, "http")
		c.SwitchTab("discussion")
	})
	acceptanceWait(t, c, "connected", func() bool { return c.state.connected("ws") })
	for _, char := range "no-echo-draft" {
		screen.InjectKey(tcell.KeyRune, char, tcell.ModNone)
	}
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	acceptanceWait(t, c, "timeout feedback", func() bool { return strings.Contains(c.state.StatusBar.GetText(true), "server did not confirm") })
	screen.InjectKey(tcell.KeyF5, 0, tcell.ModNone)
	acceptanceWait(t, c, "navigation after timeout", func() bool { return c.currentTab == "help" })
	uiDo(t, c, func() {
		if c.focus["discussion"][0].(*tview.InputField).GetText() != "no-echo-draft" || !c.state.connected("ws") {
			t.Error("missing echo erased the draft or disconnected an otherwise healthy socket")
		}
	})
}
