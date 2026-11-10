package tui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type UIController struct {
	state      *AppState
	api        *APIClient
	tcp        *TCPWorker
	udp        *UDPWorker
	chat       *ChatClient
	headerNav  *tview.TextView
	currentTab string
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
		c.state.Token = sess.Token
		c.state.UserID = sess.UserID
		c.state.Username = sess.Username

		c.onLoginSuccess()
		c.state.SetStatus(fmt.Sprintf("[green]Restored session for %s from .tui_session.json[white]", sess.Username))
	} else {
		// Default to browser view for guest exploration
		c.SwitchTab("browser")
	}
}

func (c *UIController) SetupLayout() tview.Primitive {
	// 1. Top Header Navigation Bar
	c.headerNav = tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter)
	c.updateNavHeader()

	// 2. Bottom Status Bar with live protocol indicators
	c.state.StatusBar = tview.NewTextView().
		SetDynamicColors(true).
		SetText(" Ready. Browsing as Guest. Press [yellow]'0'[white] or [yellow]F10[white] to log in.")

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
		AddItem(c.headerNav, 1, 0, false).
		AddItem(c.state.Pages, 0, 1, true).
		AddItem(c.state.StatusBar, 1, 0, false)

	// 5. Global Keyboard Shortcuts
	c.state.App.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// Allow dialogs and modals to handle their own keys
		if c.state.Pages.HasPage("dialog") || c.state.Pages.HasPage("update_dialog") {
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

		// Exit on Alt+Q
		if event.Modifiers()&tcell.ModAlt != 0 && (event.Rune() == 'q' || event.Rune() == 'Q') {
			c.state.App.Stop()
			return nil
		}

		// Number shortcuts (when not focused on an input field)
		currentFocus := c.state.App.GetFocus()
		if _, isInput := currentFocus.(*tview.InputField); !isInput {
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
			case '0', 'l', 'L':
				c.SwitchTab("auth")
				return nil
			case '?':
				c.SwitchTab("help")
				return nil
			}

			// Tab / Backtab cycling
			if event.Key() == tcell.KeyTab {
				c.cycleTab(1)
				return nil
			} else if event.Key() == tcell.KeyBacktab {
				c.cycleTab(-1)
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
	parts = append(parts, "[black:darkcyan:b] 📚 MANGAHUB TUI [-:-:-]")

	for _, t := range tabOrder {
		if t.ID == c.currentTab {
			parts = append(parts, fmt.Sprintf("[black:yellow:b] [%s] %s [-:-:-]", t.Number, t.Name))
		} else {
			parts = append(parts, fmt.Sprintf("[white:blue] [%s] %s [-:-:-]", t.Number, t.Name))
		}
	}

	userBadge := "[gray]Guest[white]"
	if c.state.Username != "" {
		userBadge = fmt.Sprintf("[cyan:b]👤 %s[-:-:-]", c.state.Username)
	}
	parts = append(parts, fmt.Sprintf(" %s", userBadge))

	c.headerNav.SetText(" " + strings.Join(parts, "  "))
}

func (c *UIController) SwitchTab(tab string) {
	c.currentTab = tab
	c.updateNavHeader()
	c.state.Pages.SwitchToPage(tab)

	switch tab {
	case "browser":
		if len(c.state.MangaCatalog) == 0 {
			c.refreshMangaCatalog("")
		}
		if c.state.CatalogTable != nil {
			c.state.App.SetFocus(c.state.CatalogTable)
		}
	case "library":
		c.refreshLibrary()
		if c.state.LibraryTable != nil {
			c.state.App.SetFocus(c.state.LibraryTable)
		}
	case "discussion":
		if c.state.Username != "" && !c.state.WSConnected {
			_ = c.chat.Connect(c.state.ActiveRoom)
		}
	case "notifications":
		c.renderNotificationsList()
		c.state.SetStatus("[yellow]Viewing UDP chapter release announcements (port 9091)[white]")
	case "help":
		c.state.SetStatus("[cyan]Viewing system architecture & protocol diagnostics[white]")
	case "auth":
		c.state.Pages.AddPage("auth", c.buildAuthView(), true, true)
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
	return tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(p, height, 1, true).
			AddItem(nil, 0, 1, false), width, 1, true).
		AddItem(nil, 0, 1, false)
}
