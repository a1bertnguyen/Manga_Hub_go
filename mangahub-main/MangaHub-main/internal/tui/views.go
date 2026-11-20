package tui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type UIController struct {
	state          *AppState
	api            *APIClient
	tcp            *TCPWorker
	udp            *UDPWorker
	chat           *ChatClient
	headerNav      *tview.TextView
	currentTab     string
	footer         *tview.TextView
	focus          map[string][]tview.Primitive
	genre          string
	search         string
	catalogRequest uint64
	detailRequest  uint64
}

func NewUIController(state *AppState, api *APIClient, tcp *TCPWorker, udp *UDPWorker, chat *ChatClient) *UIController {
	return &UIController{
		state:      state,
		api:        api,
		tcp:        tcp,
		udp:        udp,
		chat:       chat,
		currentTab: "browser",
	}
}

// RestoreSession attempts to load credentials saved in previous sessions
func (c *UIController) RestoreSession() {
	sess, err := LoadSession()
	if err == nil && sess != nil && sess.Token != "" {
		c.state.setSession(sess.Token, sess.UserID, sess.Username)

		c.onLoginSuccess()
		c.state.SetStatus(fmt.Sprintf("[green]Restored session for %s from .tui_session.json[white]", sess.Username))
	} else {
		// Default to auth tab for clean login / register / guest experience
		c.SwitchTab("auth")
	}
}

func (c *UIController) SetupLayout() tview.Primitive {
	applyTheme()
	c.focus = make(map[string][]tview.Primitive)
	// 1. Top Header Navigation Bar
	c.headerNav = tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true)
	c.headerNav.SetBackgroundColor(colorPanel)
	c.headerNav.SetHighlightedFunc(func(added, removed, remaining []string) {
		if len(added) > 0 {
			c.SwitchTab(added[0])
			c.headerNav.Highlight()
		}
	})
	c.updateNavHeader()

	// 2. Bottom Status Bar with live protocol indicators
	c.state.StatusBar = tview.NewTextView().
		SetDynamicColors(true).
		SetText(" Ready. Open Account to sign in and start your reading journey.")
	c.state.StatusBar.SetBackgroundColor(colorPanel)
	c.footer = hint("F1-F5 / F10  Navigate   Tab  Next control   Alt+Left/Right  Switch page   Ctrl+C  Quit")

	// 3. Central Tabbed Pages
	if c.state.Pages == nil {
		c.state.Pages = tview.NewPages()
	}
	c.state.Pages.AddPage("browser", c.buildBrowserView(), true, true)
	c.state.Pages.AddPage("library", c.buildLibraryView(), true, false)
	c.state.Pages.AddPage("discussion", c.buildDiscussionView(), true, false)
	c.state.Pages.AddPage("notifications", c.buildNotificationsView(), true, false)
	c.state.Pages.AddPage("help", c.buildHelpView(), true, false)
	c.state.Pages.AddPage("auth", c.buildAuthView(), true, false)

	// 4. Root Flex Container
	root := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(c.headerNav, 3, 0, false).
		AddItem(c.state.Pages, 0, 1, true).
		AddItem(c.state.StatusBar, 2, 0, false).
		AddItem(c.footer, 1, 0, false)

	// 5. Global Keyboard Shortcuts
	c.state.App.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyCtrlC || (event.Modifiers()&tcell.ModAlt != 0 && (event.Rune() == 'q' || event.Rune() == 'Q')) {
			c.state.App.Stop()
			return nil
		}
		// Allow dialogs and modals to handle their own keys
		if c.state.Pages.HasPage("dialog") || c.state.Pages.HasPage("update_dialog") {
			return event
		}
		if event.Modifiers()&tcell.ModAlt != 0 {
			if event.Key() == tcell.KeyRight {
				c.cycleTab(1)
				return nil
			}
			if event.Key() == tcell.KeyLeft {
				c.cycleTab(-1)
				return nil
			}
		}
		if event.Key() == tcell.KeyTab || event.Key() == tcell.KeyBacktab {
			// Forms own their internal focus order, including dropdowns and buttons.
			if c.currentTab != "auth" {
				delta := 1
				if event.Key() == tcell.KeyBacktab {
					delta = -1
				}
				c.focusControl(delta)
				return nil
			}
			return event
		}

		// Function Keys
		switch event.Key() {
		case tcell.KeyF1:
			c.SwitchTab("browser")
			return nil
		case tcell.KeyF2:
			c.SwitchTab("library")
			return nil
		case tcell.KeyF3:
			c.SwitchTab("discussion")
			return nil
		case tcell.KeyF4:
			c.SwitchTab("notifications")
			return nil
		case tcell.KeyF5:
			c.SwitchTab("help")
			return nil
		case tcell.KeyF10:
			c.SwitchTab("auth")
			return nil
		}

		// Number shortcuts (when not focused on an input field)
		currentFocus := c.state.App.GetFocus()
		_, isInput := currentFocus.(*tview.InputField)
		_, isDropDown := currentFocus.(*tview.DropDown)
		if !isInput && !isDropDown && c.currentTab != "auth" {
			switch event.Rune() {
			case '1':
				c.SwitchTab("browser")
				return nil
			case '2':
				c.SwitchTab("library")
				return nil
			case '3':
				c.SwitchTab("discussion")
				return nil
			case '4':
				c.SwitchTab("notifications")
				return nil
			case '5':
				c.SwitchTab("help")
				return nil
			case '0':
				c.SwitchTab("auth")
				return nil
			case '?':
				c.SwitchTab("help")
				return nil
			}
		}

		return event
	})

	return root
}

var tabOrder = []struct {
	Key    string
	Number string
	Name   string
	ID     string
}{
	{"F1", "1", "Catalog", "browser"},
	{"F2", "2", "Library", "library"},
	{"F3", "3", "Discussion", "discussion"},
	{"F4", "4", "Alerts", "notifications"},
	{"F5", "5", "System", "help"},
	{"F10", "0", "Account", "auth"},
}

func (c *UIController) updateNavHeader() {
	var parts []string

	for _, t := range tabOrder {
		if t.ID == c.currentTab {
			parts = append(parts, fmt.Sprintf("[\"%s\"][#0b1220:#5eead4:b] %s %s [-:-:-][\"\"]", t.ID, t.Number, t.Name))
		} else {
			parts = append(parts, fmt.Sprintf("[\"%s\"][#8fa4bd] %s %s [-][\"\"]", t.ID, t.Number, t.Name))
		}
	}

	userBadge := "[#8fa4bd]Sign in to sync your library[-]"
	if c.state.username() != "" {
		userBadge = fmt.Sprintf("[#5eead4]%s[-]", tview.Escape(c.state.username()))
	}
	c.headerNav.SetText(" [#5eead4::b]M A N G A H U B[-:-:-]  [#8fa4bd]YOUR READING SPACE[-]   " + userBadge + "\n\n " + strings.Join(parts, " "))
}

func (c *UIController) SwitchTab(tab string) {
	if !c.state.Pages.HasPage(tab) {
		return
	}
	c.currentTab = tab
	c.updateNavHeader()
	c.state.Pages.SwitchToPage(tab)
	if controls := c.focus[tab]; len(controls) > 0 {
		c.state.App.SetFocus(controls[0])
	}
	hints := map[string]string{
		"browser":       "/ Search   g Genre   Enter Add to library   r Refresh   Tab Details",
		"library":       "Enter Edit progress   + Next chapter   - Previous chapter   r Refresh",
		"discussion":    "Tab Rooms / Conversation / Message   Enter Join room or send message",
		"notifications": "s Subscribe   u Unsubscribe   t Demo release   c Clear alerts",
		"help":          "r Refresh diagnostics   Up/Down Scroll   F1-F5 / F10 Navigate",
		"auth":          "Tab / Shift+Tab Move   Enter Submit   F1-F5 Navigate   Ctrl+C Quit",
	}
	if c.footer != nil {
		c.footer.SetText(" " + hints[tab])
	}

	switch tab {
	case "browser":
		if len(c.state.MangaCatalog) == 0 {
			c.refreshMangaCatalog("")
		}
		if c.state.CatalogTable != nil {
			c.state.App.SetFocus(c.state.CatalogTable)
		}
	case "library":
		if c.state.username() == "" {
			populateLibraryTable(c.state.LibraryTable, nil, nil)
		}
		c.refreshLibrary()
		if c.state.LibraryTable != nil {
			c.state.App.SetFocus(c.state.LibraryTable)
		}
	case "discussion":
		if c.state.username() != "" && !c.state.connected("ws") {
			c.state.ChatMessagesView.SetText("[#8fa4bd]Joining #" + tview.Escape(c.state.ActiveRoom) + "...[-]\n\n")
			go func() {
				_ = c.chat.Connect(c.state.ActiveRoom)
			}()
		}
	case "notifications":
		c.renderNotificationsList()
		c.state.SetStatus("[yellow]Viewing UDP chapter release announcements (port 9091)[white]")
	case "help":
		if controls := c.focus[tab]; len(controls) > 0 {
			c.renderHelpContent(controls[0].(*tview.TextView))
		}
		c.state.SetStatus("[cyan]Viewing system architecture & protocol diagnostics[white]")
	case "auth":
		c.state.Pages.AddPage("auth", c.buildAuthView(), true, true)
		c.state.App.SetFocus(c.focus["auth"][0])
		c.state.SetStatus("[yellow]Viewing account & session settings[white]")
	}
}

func (c *UIController) cycleTab(delta int) {
	currentIdx := 0
	for i, t := range tabOrder {
		if t.ID == c.currentTab {
			currentIdx = i
			break
		}
	}

	nextIdx := (currentIdx + delta + len(tabOrder)) % len(tabOrder)
	c.SwitchTab(tabOrder[nextIdx].ID)
}

// centerPrimitive centers a component on screen within given width and height limits
func centerPrimitive(p tview.Primitive, width, height int) tview.Primitive {
	return &centeredPrimitive{Box: tview.NewBox(), child: p, width: width, height: height}
}

func (c *UIController) focusControl(delta int) {
	controls := c.focus[c.currentTab]
	if len(controls) == 0 {
		return
	}
	index := -1
	for i, control := range controls {
		if control.HasFocus() {
			index = i
			break
		}
	}
	if index < 0 {
		index = 0
	} else {
		index = (index + delta + len(controls)) % len(controls)
	}
	c.state.App.SetFocus(controls[index])
}

type centeredPrimitive struct {
	*tview.Box
	child         tview.Primitive
	width, height int
}

func (p *centeredPrimitive) Draw(screen tcell.Screen) {
	p.Box.Draw(screen)
	x, y, width, height := p.GetInnerRect()
	w, h := min(width, p.width), min(height, p.height)
	p.child.SetRect(x+(width-w)/2, y+(height-h)/2, w, h)
	p.child.Draw(screen)
}

func (p *centeredPrimitive) Focus(delegate func(tview.Primitive)) { delegate(p.child) }
func (p *centeredPrimitive) HasFocus() bool                       { return p.child.HasFocus() }

// tview dispatches events from the root through every container, even when the
// focused primitive is a nested field. Box's default handlers do nothing.
func (p *centeredPrimitive) InputHandler() func(*tcell.EventKey, func(tview.Primitive)) {
	return p.WrapInputHandler(p.child.InputHandler())
}
func (p *centeredPrimitive) PasteHandler() func(string, func(tview.Primitive)) {
	return p.WrapPasteHandler(p.child.PasteHandler())
}
func (p *centeredPrimitive) MouseHandler() func(tview.MouseAction, *tcell.EventMouse, func(tview.Primitive)) (bool, tview.Primitive) {
	return p.child.MouseHandler()
}
