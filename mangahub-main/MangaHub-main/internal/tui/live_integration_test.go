package tui

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// scripts/verify_connected.py owns the separate real backend/catalog processes
// and supplies temporary credentials. No saved user session is read or written.
func TestLiveConnectedTUI(t *testing.T) {
	if os.Getenv("MANGAHUB_LIVE_API") == "" {
		t.Skip("run python scripts/verify_connected.py --with-tui")
	}
	newClient := func(account string) *UIController {
		app := tview.NewApplication()
		screen := tcell.NewSimulationScreen("UTF-8")
		app.SetScreen(screen)
		screen.SetSize(110, 34)
		s := NewAppState(app, tview.NewPages(), os.Getenv("MANGAHUB_LIVE_API"), os.Getenv("MANGAHUB_LIVE_TCP"), os.Getenv("MANGAHUB_LIVE_UDP"), os.Getenv("MANGAHUB_LIVE_WS"))
		s.Token = os.Getenv("MANGAHUB_LIVE_" + account + "_TOKEN")
		s.UserID = os.Getenv("MANGAHUB_LIVE_" + account + "_ID")
		s.Username = os.Getenv("MANGAHUB_LIVE_" + account + "_NAME")
		c := NewUIController(s, NewAPIClient(s), NewTCPWorker(s), NewUDPWorker(s), NewChatClient(s))
		root := c.SetupLayout()
		app.SetRoot(root, true)
		ready, done := make(chan struct{}), make(chan error, 1)
		var once sync.Once
		app.SetAfterDrawFunc(func(tcell.Screen) { once.Do(func() { close(ready) }) })
		go func() { done <- app.Run() }()
		select {
		case <-ready:
		case <-time.After(3 * time.Second):
			t.Fatal("live TUI failed to start")
		}
		t.Cleanup(func() {
			c.tcp.Stop()
			c.udp.Stop()
			c.chat.Close()
			app.Stop()
			select {
			case err := <-done:
				if err != nil {
					t.Error(err)
				}
			case <-time.After(3 * time.Second):
				t.Error("live TUI did not stop")
			}
		})
		uiDo(t, c, func() { c.SwitchTab("library") })
		c.tcp.Start()
		return c
	}
	a, b, other := newClient("A"), newClient("A"), newClient("B")
	waitFor := func(label string, c *UIController, predicate func() bool) {
		t.Helper()
		deadline := time.Now().Add(6 * time.Second)
		for time.Now().Before(deadline) {
			ok := false
			uiDo(t, c, func() { ok = predicate() })
			if ok {
				return
			}
			time.Sleep(15 * time.Millisecond)
		}
		t.Fatalf("%s did not arrive in live TUI", label)
	}
	for _, c := range []*UIController{a, b, other} {
		waitFor("authenticated TCP", c, func() bool { return c.state.connected("tcp") })
	}
	id := os.Getenv("MANGAHUB_LIVE_MANGA")
	if err := a.api.UpdateProgress(id, 2, "reading"); err != nil {
		t.Fatal(err)
	}
	waitFor("TCP progress without refresh", b, func() bool {
		b.state.mu.RLock()
		defer b.state.mu.RUnlock()
		return len(b.state.UserLibrary) > 0 && b.state.UserLibrary[0].CurrentChapter == 2 && strings.Contains(b.state.LibraryTable.GetCell(1, 1).Text, "2")
	})
	uiDo(t, other, func() {
		other.state.mu.RLock()
		defer other.state.mu.RUnlock()
		if len(other.state.UserLibrary) != 0 {
			t.Error("another account received private progress")
		}
	})
	t.Log("PASS: live TCP update rendered in second TUI without refresh; other account isolated")
	a.udp.Start()
	b.udp.Start()
	for _, c := range []*UIController{a, b} {
		waitFor("UDP subscription", c, func() bool { return c.state.connected("udp") })
	}
	total, _ := strconv.Atoi(os.Getenv("MANGAHUB_LIVE_TOTAL"))
	if err := a.api.TriggerRelease(id, total+2, "Live TUI verification"); err != nil {
		t.Fatal(err)
	}
	for _, c := range []*UIController{a, b} {
		waitFor("visible UDP alert", c, func() bool { return strings.Contains(c.state.NotifView.GetText(true), "Live TUI verification") })
	}
	b.udp.Stop()
	before := 0
	uiDo(t, b, func() { b.state.mu.RLock(); before = len(b.state.NotificationList); b.state.mu.RUnlock() })
	if err := a.api.TriggerRelease(id, total+3, "Subscribed only"); err != nil {
		t.Fatal(err)
	}
	waitFor("second UDP alert", a, func() bool { return strings.Contains(a.state.NotifView.GetText(true), "Subscribed only") })
	time.Sleep(150 * time.Millisecond)
	uiDo(t, b, func() {
		b.state.mu.RLock()
		defer b.state.mu.RUnlock()
		if len(b.state.NotificationList) != before {
			t.Error("unsubscribed TUI received an alert")
		}
	})
	t.Log("PASS: two TUIs rendered UDP alerts; unsubscribed TUI stopped receiving them")
	for i, c := range []*UIController{a, b, other} {
		room := "live-review"
		if i == 2 {
			room = "isolated-review"
		}
		uiDo(t, c, func() { c.state.ActiveRoom = room })
		if err := c.chat.Connect(room); err != nil {
			t.Fatal(err)
		}
	}
	marker := fmt.Sprintf("live chat %d", time.Now().UnixNano())
	if err := a.chat.SendMessage(marker); err != nil {
		t.Fatal(err)
	}
	waitFor("WebSocket chat", b, func() bool { return strings.Contains(b.state.ChatMessagesView.GetText(true), marker) })
	time.Sleep(150 * time.Millisecond)
	uiDo(t, other, func() {
		if strings.Contains(other.state.ChatMessagesView.GetText(true), marker) {
			t.Error("room isolation failed in TUI")
		}
	})
	b.chat.Close()
	if err := a.chat.SendMessage("after peer disconnect"); err != nil {
		t.Fatal(err)
	}
	waitFor("chat after peer disconnect", a, func() bool { return strings.Contains(a.state.ChatMessagesView.GetText(true), "after peer disconnect") })
	uiDo(t, a, func() {
		uiKey(a, tcell.KeyF4, 0)
		if a.currentTab != "notifications" {
			t.Error("network traffic blocked navigation")
		}
	})
	t.Log("PASS: authenticated chat rendered; rooms isolated; disconnect and keyboard navigation remained functional")
}
