package tui

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func waitRootInput(t *testing.T, c *UIController, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		ok := false
		uiDo(t, c, func() { ok = check() })
		if ok {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("terminal event did not reach the visible control through the root layout")
}

func TestRootDispatchAccountTyping(t *testing.T) {
	t.Chdir(t.TempDir())
	c, screen := newTestUI(t, nil)
	uiDo(t, c, func() { c.RestoreSession() }) // Start on Account without pressing a shortcut.
	for _, char := range "alice123" {
		screen.InjectKey(tcell.KeyRune, char, tcell.ModNone)
	}
	waitRootInput(t, c, func() bool {
		form := c.focus["auth"][0].(*tview.Form)
		return form.GetFormItem(0).(*tview.InputField).GetText() == "alice123"
	})
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	for _, char := range "pass1234" {
		screen.InjectKey(tcell.KeyRune, char, tcell.ModNone)
	}
	waitRootInput(t, c, func() bool {
		form := c.focus["auth"][0].(*tview.Form)
		return form.GetFormItem(1).(*tview.InputField).GetText() == "pass1234"
	})
}

func TestRootMouseClickAndAccountSubmit(t *testing.T) {
	submitted := make(chan map[string]string, 1)
	c, screen := newTestUI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/login" {
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			submitted <- body
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"test response"}`))
		}
	}))
	var clickX, clickY int
	uiDo(t, c, func() {
		c.state.App.EnableMouse(true)
		cells, width, height := screen.GetContents()
		clickX = -1
		for y := 0; y < height; y++ {
			var line []rune
			for x := 0; x < width; x++ {
				r := ' '
				if len(cells[y*width+x].Runes) > 0 {
					r = cells[y*width+x].Runes[0]
				}
				line = append(line, r)
			}
			if index := strings.Index(string(line), "0 Account"); index >= 0 {
				clickX, clickY = index+2, y
				break
			}
		}
	})
	if clickX < 0 {
		t.Fatal("Account navigation label was not rendered")
	}
	screen.InjectMouse(clickX, clickY, tcell.Button1, tcell.ModNone)
	screen.InjectMouse(clickX, clickY, tcell.ButtonNone, tcell.ModNone)
	waitRootInput(t, c, func() bool { return c.currentTab == "auth" })
	for _, char := range "mouse-user" {
		screen.InjectKey(tcell.KeyRune, char, tcell.ModNone)
	}
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	for _, char := range "password123" {
		screen.InjectKey(tcell.KeyRune, char, tcell.ModNone)
	}
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone) // email
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone) // Login
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	select {
	case body := <-submitted:
		if body["username"] != "mouse-user" || body["password"] != "password123" {
			t.Fatal("visible form did not submit the typed values")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("mouse navigation followed by Tab/Enter did not submit login")
	}
}

func TestRootProgressDialogInputAndPaste(t *testing.T) {
	c, screen := newTestUI(t, nil)
	uiDo(t, c, func() {
		c.state.App.EnablePaste(true)
		c.SwitchTab("library")
		c.showUpdateProgressDialog(LibraryItem{Title: "Test", CurrentChapter: 4, Status: "reading"})
	})
	screen.InjectKey(tcell.KeyBackspace2, 0, tcell.ModNone)
	screen.InjectKey(tcell.KeyRune, '2', tcell.ModNone)
	waitRootInput(t, c, func() bool {
		return acceptanceForm(c).GetFormItem(0).(*tview.InputField).GetText() == "2"
	})
	screen.InjectKey(tcell.KeyEscape, 0, tcell.ModNone)
	waitRootInput(t, c, func() bool { return !c.state.Pages.HasPage("update_dialog") && c.state.LibraryTable.HasFocus() })
	screen.InjectKey(tcell.KeyF10, 0, tcell.ModNone)
	waitRootInput(t, c, func() bool { return c.currentTab == "auth" })
	c.state.App.QueueEvent(tcell.NewEventPaste(true))
	for _, char := range "pasted-user12" {
		c.state.App.QueueEvent(tcell.NewEventKey(tcell.KeyRune, char, tcell.ModNone))
	}
	c.state.App.QueueEvent(tcell.NewEventPaste(false))
	waitRootInput(t, c, func() bool {
		return c.focus["auth"][0].(*tview.Form).GetFormItem(0).(*tview.InputField).GetText() == "pasted-user12"
	})
}

func TestRootSearchAndDiscussionTyping(t *testing.T) {
	c, screen := newTestUI(t, nil)
	screen.InjectKey(tcell.KeyF1, 0, tcell.ModNone)
	screen.InjectKey(tcell.KeyRune, '/', tcell.ModNone)
	for _, char := range "One Piece 123" {
		screen.InjectKey(tcell.KeyRune, char, tcell.ModNone)
	}
	waitRootInput(t, c, func() bool {
		return c.currentTab == "browser" && c.focus["browser"][2].(*tview.InputField).GetText() == "One Piece 123"
	})
	screen.InjectKey(tcell.KeyF3, 0, tcell.ModNone)
	for _, char := range "Hello room 123" {
		screen.InjectKey(tcell.KeyRune, char, tcell.ModNone)
	}
	waitRootInput(t, c, func() bool {
		return c.currentTab == "discussion" && c.focus["discussion"][0].(*tview.InputField).GetText() == "Hello room 123"
	})
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	waitRootInput(t, c, func() bool { return c.focus["discussion"][1].HasFocus() })
	screen.InjectKey(tcell.KeyBacktab, 0, tcell.ModNone)
	waitRootInput(t, c, func() bool { return c.focus["discussion"][0].HasFocus() })
}
