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

func acceptanceClient(t *testing.T, token, id, name string) *UIController {
	t.Helper()
	app := tview.NewApplication()
	screen := tcell.NewSimulationScreen("UTF-8")
	app.SetScreen(screen)
	screen.SetSize(120, 38)
	s := NewAppState(app, tview.NewPages(), os.Getenv("MANGAHUB_LIVE_API"), os.Getenv("MANGAHUB_LIVE_TCP"), os.Getenv("MANGAHUB_LIVE_UDP"), os.Getenv("MANGAHUB_LIVE_WS"))
	s.setSession(token, id, name)
	c := NewUIController(s, NewAPIClient(s), NewTCPWorker(s), NewUDPWorker(s), NewChatClient(s))
	app.SetRoot(c.SetupLayout(), true)
	ready, done := make(chan struct{}), make(chan error, 1)
	var once sync.Once
	app.SetAfterDrawFunc(func(tcell.Screen) { once.Do(func() { close(ready) }) })
	go func() { done <- app.Run() }()
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("TUI did not start")
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
		case <-time.After(5 * time.Second):
			t.Error("TUI did not stop")
		}
	})
	return c
}

func acceptanceWait(t *testing.T, c *UIController, label string, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		ok := false
		uiDo(t, c, func() { ok = check() })
		if ok {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("Acceptance result was not rendered: " + label)
}

func acceptanceButton(c *UIController, form *tview.Form, index int) {
	c.state.App.SetFocus(form.GetButton(index))
	uiKey(c, tcell.KeyEnter, 0)
}

func acceptanceForm(c *UIController) *tview.Form {
	_, page := c.state.Pages.GetFrontPage()
	return page.(*centeredPrimitive).child.(*tview.Flex).GetItem(0).(*tview.Form)
}

func TestLiveFeatureAcceptanceTUI(t *testing.T) {
	if os.Getenv("MANGAHUB_LIVE_API") == "" {
		t.Skip("run python scripts/verify_postman.py")
	}
	t.Chdir(t.TempDir()) // SaveSession may only touch this test's temporary directory.
	c := acceptanceClient(t, "", "", "")
	username := fmt.Sprintf("tui_%d", time.Now().UnixNano())
	password := "Temporary-TUI-demo-password!"
	uiDo(t, c, func() {
		c.SwitchTab("auth")
		form := c.focus["auth"][0].(*tview.Form)
		form.GetFormItem(0).(*tview.InputField).SetText(username)
		form.GetFormItem(1).(*tview.InputField).SetText(password)
		form.GetFormItem(2).(*tview.InputField).SetText(username + "@example.test")
		acceptanceButton(c, form, 1)
	})
	acceptanceWait(t, c, "F01 registration", func() bool { return c.currentTab == "browser" && c.state.token() != "" })
	uiDo(t, c, func() {
		c.SwitchTab("auth")
		acceptanceButton(c, c.focus["auth"][0].(*tview.Form), 0)
		form := c.focus["auth"][0].(*tview.Form)
		form.GetFormItem(0).(*tview.InputField).SetText(username)
		form.GetFormItem(1).(*tview.InputField).SetText("wrong-password")
		acceptanceButton(c, form, 0)
	})
	acceptanceWait(t, c, "F01 wrong-password feedback", func() bool {
		_, page := c.state.Pages.GetFrontPage()
		feedback := page.(*centeredPrimitive).child.(*tview.Flex).GetItem(2).(*tview.TextView)
		return strings.Contains(feedback.GetText(true), "Login failed") && c.state.token() == ""
	})
	uiDo(t, c, func() {
		form := c.focus["auth"][0].(*tview.Form)
		form.GetFormItem(1).(*tview.InputField).SetText(password)
		acceptanceButton(c, form, 0)
	})
	acceptanceWait(t, c, "F01 successful login", func() bool { return c.currentTab == "browser" && len(c.state.MangaCatalog) > 0 })
	t.Log("F01 PASS: actual Register/Login/Logout buttons and wrong-password feedback through the running TUI")

	var title, id, author, description, genre, status string
	var total int
	uiDo(t, c, func() {
		m := c.state.MangaCatalog[0]
		title, id, author, description, status, total = m.Title, m.ID, m.Author, m.Description, m.Status, m.TotalChapters
		if len(m.Genres) > 0 {
			genre = m.Genres[0]
		}
		search := c.focus["browser"][2].(*tview.InputField)
		search.SetText(title)
		c.state.App.SetFocus(search)
		uiKey(c, tcell.KeyEnter, 0)
	})
	acceptanceWait(t, c, "F02 title search and all detail fields", func() bool {
		text := c.state.DetailView.GetText(true)
		return len(c.state.MangaCatalog) > 0 && strings.Contains(text, title) && strings.Contains(text, author) && strings.Contains(text, description) && strings.Contains(text, genre) && strings.Contains(text, strings.ToUpper(status)) && strings.Contains(text, strconv.Itoa(total))
	})
	uiDo(t, c, func() {
		search := c.focus["browser"][2].(*tview.InputField)
		search.SetText("__missing_tui_acceptance_91738__")
		c.state.App.SetFocus(search)
		uiKey(c, tcell.KeyEnter, 0)
	})
	acceptanceWait(t, c, "F02 no-result feedback", func() bool {
		return len(c.state.MangaCatalog) == 0 && strings.Contains(c.state.DetailView.GetText(true), "No stories found")
	})
	uiDo(t, c, func() {
		search := c.focus["browser"][2].(*tview.InputField)
		search.SetText(title)
		c.state.App.SetFocus(search)
		uiKey(c, tcell.KeyEnter, 0)
	})
	acceptanceWait(t, c, "F02 search recovery", func() bool { return len(c.state.MangaCatalog) > 0 })
	t.Log("F02 PASS: title search, selected metadata (all six fields), and visible no-result state over real HTTP -> gRPC")

	uiDo(t, c, func() {
		c.state.CatalogTable.Select(1, 0)
		c.state.App.SetFocus(c.state.CatalogTable)
		uiKey(c, tcell.KeyEnter, 0)
		_, page := c.state.Pages.GetFrontPage()
		modal := page.(*tview.Modal)
		modal.SetFocus(1)
		c.state.App.SetFocus(modal)
		uiKey(c, tcell.KeyEnter, 0)
	})
	acceptanceWait(t, c, "F03 plan-to-read persisted", func() bool {
		c.state.mu.RLock()
		defer c.state.mu.RUnlock()
		return len(c.state.UserLibrary) == 1 && c.state.UserLibrary[0].Status == "plan_to_read"
	})
	peer := acceptanceClient(t, c.state.token(), c.state.userID(), c.state.username())
	uiDo(t, peer, func() { peer.onLoginSuccess(); peer.SwitchTab("library") })
	acceptanceWait(t, peer, "peer authenticated TCP", func() bool { return peer.state.connected("tcp") })
	acceptanceWait(t, peer, "initial library snapshot before any update", func() bool {
		peer.state.mu.RLock()
		defer peer.state.mu.RUnlock()
		return len(peer.state.UserLibrary) == 1 && peer.state.UserLibrary[0].CurrentChapter == 0 && peer.state.UserLibrary[0].Status == "plan_to_read"
	})
	other := acceptanceClient(t, os.Getenv("MANGAHUB_LIVE_B_TOKEN"), os.Getenv("MANGAHUB_LIVE_B_ID"), os.Getenv("MANGAHUB_LIVE_B_NAME"))
	uiDo(t, other, func() { other.onLoginSuccess(); other.SwitchTab("library") })
	acceptanceWait(t, other, "different account TCP session", func() bool { return other.state.connected("tcp") })
	for _, update := range []struct {
		chapter, statusIndex int
		status               string
	}{{min(2, total), 0, "reading"}, {total, 1, "completed"}} {
		uiDo(t, c, func() {
			c.SwitchTab("library")
			c.state.LibraryTable.Select(1, 0)
			c.state.App.SetFocus(c.state.LibraryTable)
			uiKey(c, tcell.KeyEnter, 0)
			form := acceptanceForm(c)
			form.GetFormItem(0).(*tview.InputField).SetText(strconv.Itoa(update.chapter))
			form.GetFormItem(1).(*tview.DropDown).SetCurrentOption(update.statusIndex)
			acceptanceButton(c, form, 0)
		})
		acceptanceWait(t, c, "F03 saved "+update.status, func() bool {
			c.state.mu.RLock()
			defer c.state.mu.RUnlock()
			return !c.state.Pages.HasPage("update_dialog") && c.state.UserLibrary[0].CurrentChapter == update.chapter && c.state.UserLibrary[0].Status == update.status
		})
		acceptanceWait(t, peer, "F03 rendered TCP peer progress", func() bool {
			peer.state.mu.RLock()
			defer peer.state.mu.RUnlock()
			return len(peer.state.UserLibrary) > 0 && peer.state.UserLibrary[0].CurrentChapter == update.chapter && strings.Contains(peer.state.LibraryTable.GetCell(1, 1).Text, strconv.Itoa(update.chapter))
		})
	}
	stored, err := c.api.FetchLibrary()
	if err != nil || len(stored) != 1 || stored[0].CurrentChapter != total || stored[0].Status != "completed" {
		t.Fatalf("F03 stored state mismatch: %v", err)
	}
	version := stored[0].Version
	uiDo(t, c, func() {
		c.state.App.SetFocus(c.state.LibraryTable)
		uiKey(c, tcell.KeyEnter, 0)
		form := acceptanceForm(c)
		form.GetFormItem(0).(*tview.InputField).SetText(strconv.Itoa(total + 1))
		acceptanceButton(c, form, 0)
	})
	acceptanceWait(t, c, "F03 invalid chapter feedback", func() bool {
		_, p := c.state.Pages.GetFrontPage()
		feedback := p.(*centeredPrimitive).child.(*tview.Flex).GetItem(1).(*tview.TextView)
		return strings.Contains(feedback.GetText(true), "HTTP 400")
	})
	stored, err = c.api.FetchLibrary()
	if err != nil || stored[0].Version != version || stored[0].CurrentChapter != total {
		t.Fatal("Invalid chapter mutated stored progress")
	}
	uiDo(t, c, func() { uiKey(c, tcell.KeyEscape, 0) })
	uiDo(t, other, func() {
		other.state.mu.RLock()
		defer other.state.mu.RUnlock()
		if len(other.state.UserLibrary) != 0 {
			t.Error("Different account received private TCP progress")
		}
	})
	t.Log("F03 PASS: add dialog, all three statuses, progress edit, peer redraw without refresh, different-account isolation, invalid chapter error and unchanged stored version")

	for _, client := range []*UIController{c, peer} {
		acceptanceWait(t, client, "UDP subscription", func() bool { return client.state.connected("udp") })
	}
	uiDo(t, other, func() { other.SwitchTab("notifications"); uiKey(other, tcell.KeyRune, 'u') })
	uiDo(t, c, func() { c.SwitchTab("notifications"); uiKey(c, tcell.KeyRune, 't') })
	for _, client := range []*UIController{c, peer} {
		acceptanceWait(t, client, "F04 notification rendered", func() bool { return strings.Contains(client.state.NotifView.GetText(true), title) })
	}
	uiDo(t, peer, func() { peer.SwitchTab("notifications"); uiKey(peer, tcell.KeyRune, 'u') })
	uiDo(t, c, func() { c.SwitchTab("browser"); c.refreshMangaCatalog(title) })
	acceptanceWait(t, c, "updated total for second release", func() bool { return len(c.state.MangaCatalog) > 0 && c.state.MangaCatalog[0].TotalChapters > total })
	uiDo(t, c, func() { c.SwitchTab("notifications"); uiKey(c, tcell.KeyRune, 't') })
	acceptanceWait(t, c, "second release", func() bool { c.state.mu.RLock(); defer c.state.mu.RUnlock(); return len(c.state.NotificationList) >= 2 })
	time.Sleep(150 * time.Millisecond)
	uiDo(t, peer, func() {
		peer.state.mu.RLock()
		defer peer.state.mu.RUnlock()
		if len(peer.state.NotificationList) != 1 {
			t.Error("Unsubscribed TUI received another notification")
		}
	})
	uiDo(t, other, func() {
		other.state.mu.RLock()
		defer other.state.mu.RUnlock()
		if len(other.state.NotificationList) != 0 {
			t.Error("Initially unsubscribed third TUI received chapter alerts")
		}
	})
	t.Log("F04 PASS: user-triggered release and unsubscribe keys; notifications rendered in two TUIs, unsubscribed peer receives no second alert")

	for _, client := range []*UIController{c, peer, other} {
		uiDo(t, client, func() { client.SwitchTab("discussion") })
		acceptanceWait(t, client, "chat connected", func() bool { return client.state.connected("ws") })
	}
	uiDo(t, other, func() {
		rooms := other.focus["discussion"][1].(*tview.List)
		rooms.SetCurrentItem(3)
		other.state.App.SetFocus(rooms)
		uiKey(other, tcell.KeyEnter, 0)
	})
	acceptanceWait(t, other, "other room joined", func() bool { return strings.Contains(other.state.ChatMessagesView.GetTitle(), "spoilers") })
	marker := fmt.Sprintf("acceptance-%d", time.Now().UnixNano())
	uiDo(t, c, func() {
		input := c.focus["discussion"][0].(*tview.InputField)
		input.SetText(marker)
		c.state.App.SetFocus(input)
		uiKey(c, tcell.KeyEnter, 0)
	})
	acceptanceWait(t, peer, "F05 received message", func() bool { return strings.Contains(peer.state.ChatMessagesView.GetText(true), marker) })
	uiDo(t, peer, func() {
		input := peer.focus["discussion"][0].(*tview.InputField)
		input.SetText("reply-" + marker)
		peer.state.App.SetFocus(input)
		uiKey(peer, tcell.KeyEnter, 0)
	})
	acceptanceWait(t, c, "F05 received reply", func() bool { return strings.Contains(c.state.ChatMessagesView.GetText(true), "reply-"+marker) })
	time.Sleep(150 * time.Millisecond)
	uiDo(t, other, func() {
		if strings.Contains(other.state.ChatMessagesView.GetText(true), marker) {
			t.Error("Other room received general chat")
		}
	})
	peer.chat.Close()
	uiDo(t, c, func() {
		input := c.focus["discussion"][0].(*tview.InputField)
		input.SetText("after-disconnect-" + marker)
		c.state.App.SetFocus(input)
		uiKey(c, tcell.KeyEnter, 0)
	})
	acceptanceWait(t, c, "F05 hub still active", func() bool {
		return strings.Contains(c.state.ChatMessagesView.GetText(true), "after-disconnect-"+marker)
	})
	uiDo(t, c, func() {
		uiKey(c, tcell.KeyF5, 0)
		if c.currentTab != "help" {
			t.Error("Network traffic blocked the TUI")
		}
	})
	t.Log("F05 PASS: actual room selection and send input, bidirectional rendered chat, room isolation, peer disconnect and responsive navigation")
	_ = id
}

func TestLiveCatalogUnavailableTUI(t *testing.T) {
	if os.Getenv("MANGAHUB_LIVE_CATALOG_DOWN") != "true" {
		t.Skip("run python scripts/verify_postman.py")
	}
	c := acceptanceClient(t, os.Getenv("MANGAHUB_LIVE_A_TOKEN"), os.Getenv("MANGAHUB_LIVE_A_ID"), os.Getenv("MANGAHUB_LIVE_A_NAME"))
	uiDo(t, c, func() { c.SwitchTab("browser") })
	acceptanceWait(t, c, "F02/F06 controlled unavailable error", func() bool { return strings.Contains(c.state.DetailView.GetText(true), "HTTP 503") })
	uiDo(t, c, func() {
		uiKey(c, tcell.KeyF5, 0)
		if c.currentTab != "help" {
			t.Error("Catalog failure froze TUI")
		}
	})
	t.Log("F02/F06 PASS: stopped real catalog -> visible HTTP 503 in TUI, keyboard navigation stays responsive")
}
