package tui

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"mangahub/pkg/models"

	"github.com/gdamore/tcell/v2"
	"github.com/gorilla/websocket"
	"github.com/rivo/tview"
)

func newTestUI(t *testing.T, handler http.Handler) (*UIController, tcell.SimulationScreen) {
	t.Helper()
	if handler == nil {
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"manga":[],"library":[],"services":{"grpc_catalog":"connected"}}`)
		})
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	app := tview.NewApplication()
	screen := tcell.NewSimulationScreen("UTF-8")
	app.SetScreen(screen)
	screen.SetSize(120, 38)
	state := NewAppState(app, tview.NewPages(), server.URL, "localhost:1", "localhost:1", "ws://localhost:1/ws")
	c := NewUIController(state, NewAPIClient(state), NewTCPWorker(state), NewUDPWorker(state), NewChatClient(state))
	root := c.SetupLayout()
	ready, done := make(chan struct{}), make(chan error, 1)
	var once sync.Once
	app.SetAfterDrawFunc(func(tcell.Screen) { once.Do(func() { close(ready) }) })
	app.SetRoot(root, true)
	go func() { done <- app.Run() }()
	select {
	case <-ready:
	case <-time.After(3 * time.Second):
		t.Fatal("TUI did not start")
	}
	t.Cleanup(func() {
		app.Stop()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("TUI stopped with error: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Error("TUI did not stop")
		}
	})
	return c, screen
}

func uiDo(t *testing.T, c *UIController, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { c.state.App.QueueUpdateDraw(fn); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("TUI event loop blocked")
	}
}

func uiKey(c *UIController, key tcell.Key, char rune) {
	event := c.state.App.GetInputCapture()(tcell.NewEventKey(key, char, tcell.ModNone))
	if event != nil {
		// Follow production dispatch through the page/container hierarchy instead
		// of bypassing it by calling the focused field's handler directly.
		if handler := c.state.Pages.InputHandler(); handler != nil {
			handler(event, func(p tview.Primitive) { c.state.App.SetFocus(p) })
		}
	}
}

func TestRedesignFocusAndTextInput(t *testing.T) {
	c, _ := newTestUI(t, nil)
	uiDo(t, c, func() {
		c.SwitchTab("discussion")
		uiKey(c, tcell.KeyRune, '2')
		input := c.focus["discussion"][0].(*tview.InputField)
		if input.GetText() != "2" || c.currentTab != "discussion" {
			t.Error("numeric message text switched page")
		}
		uiKey(c, tcell.KeyTab, 0)
		if !c.focus["discussion"][1].HasFocus() || c.currentTab != "discussion" {
			t.Error("Tab must focus rooms without leaving Discussion")
		}
		uiKey(c, tcell.KeyBacktab, 0)
		if !input.HasFocus() {
			t.Error("Shift+Tab did not return to message input")
		}
		uiKey(c, tcell.KeyF10, 0)
		uiKey(c, tcell.KeyRune, '1')
		if c.currentTab != "auth" {
			t.Error("typing a username switched page")
		}
		uiKey(c, tcell.KeyTab, 0)
		uiKey(c, tcell.KeyRune, '2')
		form := c.focus["auth"][0].(*tview.Form)
		if form.GetFormItem(1).(*tview.InputField).GetText() != "2" {
			t.Error("Tab did not move to password")
		}
		uiKey(c, tcell.KeyF1, 0)
		uiKey(c, tcell.KeyRune, '/')
		uiKey(c, tcell.KeyRune, '3')
		if c.currentTab != "browser" || c.focus["browser"][2].(*tview.InputField).GetText() != "3" {
			t.Error("search text was consumed as navigation")
		}
	})
}

func TestRedesignCatalogHTTPContract(t *testing.T) {
	query := "Love & War + [red]"
	genre := "Sci-Fi & Fantasy"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") != query || r.URL.Query().Get("genre") != genre {
			t.Errorf("search filters corrupted: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, `{"error":"Catalog is offline"}`)
	}))
	defer server.Close()
	state := NewAppState(tview.NewApplication(), tview.NewPages(), server.URL, "", "", "")
	_, err := NewAPIClient(state).FetchManga(query, genre)
	if err == nil || !strings.Contains(err.Error(), "503") || !strings.Contains(err.Error(), "Catalog is offline") {
		t.Fatalf("status and error must reach the UI: %v", err)
	}
}

func TestRedesignIgnoresStaleMangaDetails(t *testing.T) {
	oldStarted, releaseOld, oldFinished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	c, _ := newTestUI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/old") {
			close(oldStarted)
			<-releaseOld
			defer close(oldFinished)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":%q,"title":%q}`, filepath.Base(r.URL.Path), filepath.Base(r.URL.Path))
	}))
	uiDo(t, c, func() { c.loadMangaDetailAsync("old", &models.Manga{Title: "old"}) })
	select {
	case <-oldStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("old request did not start")
	}
	uiDo(t, c, func() { c.loadMangaDetailAsync("new", &models.Manga{Title: "new"}) })
	close(releaseOld)
	<-oldFinished
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		uiDo(t, c, func() {
			if strings.Contains(c.state.DetailView.GetText(true), "old") {
				t.Error("a late response replaced the selected manga")
			}
		})
		time.Sleep(10 * time.Millisecond)
	}
}

func TestRedesignProgressDialogKeepsFocus(t *testing.T) {
	c, _ := newTestUI(t, nil)
	uiDo(t, c, func() {
		c.SwitchTab("library")
		c.showUpdateProgressDialog(LibraryItem{Title: "Test title", CurrentChapter: 4, Status: "reading"})
		uiKey(c, tcell.KeyRune, '5')
		if c.currentTab != "library" || !c.state.Pages.HasPage("update_dialog") {
			t.Error("dialog lost focus to global navigation")
		}
		uiKey(c, tcell.KeyEscape, 0)
		if c.state.Pages.HasPage("update_dialog") || !c.state.LibraryTable.HasFocus() {
			t.Error("Escape must close dialog and restore table focus")
		}
	})
}

func TestRedesignUDPStopUnsubscribes(t *testing.T) {
	server, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	client, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	state := NewAppState(tview.NewApplication(), tview.NewPages(), "", "", server.LocalAddr().String(), "")
	worker := NewUDPWorker(state)
	worker.running, worker.stop, worker.conn = true, make(chan struct{}), client
	worker.Stop()
	_ = server.SetReadDeadline(time.Now().Add(time.Second))
	data := make([]byte, 256)
	n, source, err := server.ReadFromUDP(data)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data[:n]), `"unsubscribe"`) || source.Port != client.LocalAddr().(*net.UDPAddr).Port {
		t.Fatalf("unsubscribe must originate from subscribed socket: %s", data[:n])
	}
}

func TestRedesignChatConnectDoesNotWaitForEventLoop(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_, _, _ = conn.ReadMessage()
	}))
	defer server.Close()
	c, _ := newTestUI(t, nil)
	uiDo(t, c, func() {
		// This regression targets the socket mutex/event-loop dependency.
		c.state.StatusBar = nil
		c.state.WSURL = "ws" + strings.TrimPrefix(server.URL, "http")
		done := make(chan error, 1)
		go func() { done <- c.chat.Connect("general") }()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("WebSocket connect failed: %v", err)
			}
			c.chat.Close()
		case <-time.After(time.Second):
			t.Error("Connect waited for the UI event loop while holding its socket mutex")
		}
	})
	c.chat.Close()
}

// Optional captures use fixture data in the real tcell renderer, not a network demo.
// TUI_CAPTURE_DIR=<absolute directory> go test ./internal/tui -run TestRedesignScreens -count=1
func TestRedesignScreens(t *testing.T) {
	c, screen := newTestUI(t, nil)
	for _, size := range [][2]int{{120, 38}, {80, 30}} {
		for _, page := range []string{"browser", "library", "discussion", "notifications", "help", "auth"} {
			uiDo(t, c, func() {
				screen.SetSize(size[0], size[1])
				c.state.Pages.SwitchToPage(page)
				c.currentTab = page
				c.updateNavHeader()
				c.footer.SetText(" F1-F5 / F10 Navigate   Tab Next control   Ctrl+C Quit")
				if page == "browser" {
					c.state.CatalogTable.SetCell(1, 0, tview.NewTableCell(" Frieren: Beyond Journey's End").SetExpansion(1).SetTextColor(colorText))
					c.state.CatalogTable.SetCell(1, 1, tview.NewTableCell("140"))
					c.state.CatalogTable.SetCell(1, 2, tview.NewTableCell(" ONGOING"))
					c.renderMangaDetails(&models.Manga{Title: "Frieren: Beyond Journey's End", Author: "Kanehito Yamada", Genres: []string{"Fantasy", "Adventure"}, Status: "ongoing", TotalChapters: 140, Description: "An elven mage retraces the journey she once shared with her companions, discovering the meaning of the time they spent together."})
				}
				if page == "library" {
					populateLibraryTable(c.state.LibraryTable, []LibraryItem{{MangaID: "frieren", Title: "Frieren", CurrentChapter: 42, Status: "reading"}}, []*models.Manga{{ID: "frieren", TotalChapters: 140}})
				}
				if page == "help" {
					c.focus[page][0].(*tview.TextView).SetText("[#5eead4::b]CONNECTION STATUS[-:-:-]\n\nNo live connection in this fixture preview.\n\nF1 Catalog / F2 Library / F3 Discussion\nF4 Alerts / F5 Connections / F10 Account\n\nTab moves between controls.\nAlt+Left / Right switches pages.")
				}
				if controls := c.focus[page]; len(controls) > 0 {
					c.state.App.SetFocus(controls[0])
				}
			})
			uiDo(t, c, func() {
				if dir := os.Getenv("TUI_CAPTURE_DIR"); dir != "" {
					captureSimulation(t, screen, dir, fmt.Sprintf("%s-%dx%d", page, size[0], size[1]))
				}
			})
		}
	}
}

func captureSimulation(t *testing.T, screen tcell.SimulationScreen, dir, name string) {
	t.Helper()
	cells, width, height := screen.GetContents()
	type cell struct {
		Text                   string
		Foreground, Background string
	}
	out := struct {
		Width, Height int
		Cells         []cell
	}{Width: width, Height: height}
	for _, c := range cells {
		fg, bg, _ := c.Style.Decompose()
		fr, fgreen, fb := fg.RGB()
		br, bgreen, bb := bg.RGB()
		out.Cells = append(out.Cells, cell{string(c.Runes), fmt.Sprintf("#%02x%02x%02x", fr, fgreen, fb), fmt.Sprintf("#%02x%02x%02x", br, bgreen, bb)})
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Error(err)
		return
	}
	data, err := json.Marshal(out)
	if err != nil {
		t.Error(err)
		return
	}
	if err := os.WriteFile(filepath.Join(dir, name+".json"), data, 0644); err != nil {
		t.Error(err)
	}
}
