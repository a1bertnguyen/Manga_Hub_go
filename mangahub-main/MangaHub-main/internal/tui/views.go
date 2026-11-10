package tui

import (
	"fmt"
	"strconv"
	"strings"

	"mangahub/pkg/models"

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

func (c *UIController) SetupLayout() tview.Primitive {
	// Top Header Navigation Bar
	c.headerNav = tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter)
	c.updateNavHeader()

	// Bottom Status Bar
	c.state.StatusBar = tview.NewTextView().
		SetDynamicColors(true).
		SetText(" Ready. Please login to synchronize progress and join live discussion.")

	// Central Pages
	c.state.Pages = tview.NewPages()
	c.state.Pages.AddPage("browser", c.buildBrowserView(), true, false)
	c.state.Pages.AddPage("library", c.buildLibraryView(), true, false)
	c.state.Pages.AddPage("discussion", c.buildDiscussionView(), true, false)
	c.state.Pages.AddPage("notifications", c.buildNotificationsView(), true, false)
	c.state.Pages.AddPage("help", c.buildHelpView(), true, false)

	// Auth Modal (starts on auth if not logged in)
	c.state.Pages.AddPage("auth", c.buildAuthModal(), true, true)

	// Root Flex Container
	root := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(c.headerNav, 1, 0, false).
		AddItem(c.state.Pages, 0, 1, true).
		AddItem(c.state.StatusBar, 1, 0, false)

	// Global Key Bindings
	c.state.App.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if c.state.Pages.HasPage("auth_active") {
			return event
		}

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
			c.showAuthModal()
			return nil
		case tcell.KeyRune:
			if event.Rune() == 'q' && event.Modifiers()&tcell.ModAlt != 0 {
				c.state.App.Stop()
				return nil
			}
		}
		return event
	})

	return root
}

func (c *UIController) updateNavHeader() {
	tabs := []struct {
		Key  string
		Name string
		ID   string
	}{
		{"F1", "Browser", "browser"},
		{"F2", "My Library", "library"},
		{"F3", "Discussion", "discussion"},
		{"F4", "Notifications", "notifications"},
		{"F5", "Help / Status", "help"},
		{"F10", "Account / Auth", "auth"},
	}

	var parts []string
	for _, t := range tabs {
		if t.ID == c.currentTab {
			parts = append(parts, fmt.Sprintf("[black:yellow:b] [%s] %s [-:-:-]", t.Key, t.Name))
		} else {
			parts = append(parts, fmt.Sprintf("[white:blue] [%s] %s [-:-:-]", t.Key, t.Name))
		}
	}

	c.headerNav.SetText(" " + strings.Join(parts, "  "))
}

func (c *UIController) SwitchTab(tab string) {
	c.currentTab = tab
	c.updateNavHeader()
	c.state.Pages.SwitchToPage(tab)

	switch tab {
	case "browser":
		c.refreshMangaCatalog("")
		c.state.App.SetFocus(c.state.CatalogTable)
	case "library":
		c.refreshLibrary()
		c.state.App.SetFocus(c.state.LibraryTable)
	case "discussion":
		if c.state.Username != "" && !c.state.WSConnected {
			_ = c.chat.Connect(c.state.ActiveRoom)
		}
	case "notifications":
		c.state.SetStatus("Viewing UDP chapter release notifications")
	case "help":
		c.state.SetStatus("Viewing network architecture and help guide")
	}
}

// ==========================================
// 1. AUTH SCREEN (Login / Register Modal)
// ==========================================
func (c *UIController) buildAuthModal() tview.Primitive {
	form := tview.NewForm()
	form.SetBorder(true).SetTitle(" MangaHub Account (Login / Register) ").SetTitleAlign(tview.AlignCenter)

	var username, email, password string
	form.AddInputField("Username", "", 24, nil, func(text string) { username = text })
	form.AddPasswordField("Password", "", 24, '*', func(text string) { password = text })
	form.AddInputField("Email (Register only)", "", 24, nil, func(text string) { email = text })

	statusText := tview.NewTextView().SetDynamicColors(true).SetTextAlign(tview.AlignCenter)

	form.AddButton("Login", func() {
		if username == "" || password == "" {
			statusText.SetText("[red]Username and password are required")
			return
		}
		statusText.SetText("[yellow]Authenticating...")
		go func() {
			err := c.api.Login(username, password)
			c.state.App.QueueUpdateDraw(func() {
				if err != nil {
					statusText.SetText(fmt.Sprintf("[red]Login failed: %v", err))
					return
				}
				c.onLoginSuccess()
			})
		}()
	})

	form.AddButton("Register", func() {
		if username == "" || password == "" || email == "" {
			statusText.SetText("[red]All fields (including email) are required for registration")
			return
		}
		statusText.SetText("[yellow]Creating account...")
		go func() {
			err := c.api.Register(username, email, password)
			c.state.App.QueueUpdateDraw(func() {
				if err != nil {
					statusText.SetText(fmt.Sprintf("[red]Registration failed: %v", err))
					return
				}
				c.onLoginSuccess()
			})
		}()
	})

	form.AddButton("Guest (Browse Only)", func() {
		c.state.Pages.SwitchToPage("browser")
		c.currentTab = "browser"
		c.updateNavHeader()
		c.refreshMangaCatalog("")
	})

	box := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(form, 12, 0, true).
		AddItem(statusText, 2, 0, false)

	return centerPrimitive(box, 60, 16)
}

func (c *UIController) showAuthModal() {
	c.state.Pages.SwitchToPage("auth")
	c.currentTab = "auth"
	c.updateNavHeader()
}

func (c *UIController) onLoginSuccess() {
	c.state.SetStatus(fmt.Sprintf("[green]Welcome back, %s! Starting background TCP/UDP/WS services...[white]", c.state.Username))

	// Start background workers
	c.tcp.Start()
	c.udp.Start()

	// Auto switch to browser tab
	c.SwitchTab("browser")
}

// ==========================================
// 2. MANGA BROWSER & DETAILS SCREEN (HTTP -> gRPC)
// ==========================================
func (c *UIController) buildBrowserView() tview.Primitive {
	// Search input
	searchBar := tview.NewInputField().
		SetLabel(" 🔍 Search Title: ").
		SetFieldWidth(30)

	// Catalog Table
	c.state.CatalogTable = tview.NewTable().
		SetBorders(true).
		SetSelectable(true, false)

	c.state.CatalogTable.SetCell(0, 0, tview.NewTableCell("ID").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	c.state.CatalogTable.SetCell(0, 1, tview.NewTableCell("Title").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	c.state.CatalogTable.SetCell(0, 2, tview.NewTableCell("Author").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	c.state.CatalogTable.SetCell(0, 3, tview.NewTableCell("Genres").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	c.state.CatalogTable.SetCell(0, 4, tview.NewTableCell("Chapters").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	c.state.CatalogTable.SetCell(0, 5, tview.NewTableCell("Status").SetTextColor(tcell.ColorYellow).SetSelectable(false))

	// Details & Actions Panel
	c.state.DetailView = tview.NewTextView().
		SetDynamicColors(true).
		SetWordWrap(true).
		SetText("\n Select a manga on the left to inspect details.\n Press [yellow]Enter[white] or [yellow]'a'[white] to add to personal library.")
	c.state.DetailView.SetBorder(true).SetTitle(" Manga Details (via gRPC) ")

	// Search trigger
	searchBar.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			c.refreshMangaCatalog(searchBar.GetText())
		}
	})

	// Table selection trigger (updates details view)
	c.state.CatalogTable.SetSelectionChangedFunc(func(row, column int) {
		if row <= 0 || row > len(c.state.MangaCatalog) {
			return
		}
		m := c.state.MangaCatalog[row-1]
		c.renderMangaDetails(m)
	})

	// Enter or 'a' adds selected manga to library
	c.state.CatalogTable.SetSelectedFunc(func(row, column int) {
		if row <= 0 || row > len(c.state.MangaCatalog) {
			return
		}
		m := c.state.MangaCatalog[row-1]
		c.showAddToLibraryDialog(m)
	})

	c.state.CatalogTable.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Rune() == 'a' || event.Rune() == 'A' {
			row, _ := c.state.CatalogTable.GetSelection()
			if row > 0 && row <= len(c.state.MangaCatalog) {
				c.showAddToLibraryDialog(c.state.MangaCatalog[row-1])
				return nil
			}
		}
		if event.Rune() == '/' {
			c.state.App.SetFocus(searchBar)
			return nil
		}
		return event
	})

	leftPane := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(searchBar, 1, 0, false).
		AddItem(c.state.CatalogTable, 0, 1, true)
	leftPane.SetBorder(true).SetTitle(" Manga Catalog (HTTP REST -> gRPC) ")

	split := tview.NewFlex().
		AddItem(leftPane, 0, 3, true).
		AddItem(c.state.DetailView, 0, 2, false)

	return split
}

func (c *UIController) refreshMangaCatalog(query string) {
	c.state.SetStatus(fmt.Sprintf("[yellow]Searching catalog for '%s' via gRPC...[white]", query))
	go func() {
		mangas, err := c.api.FetchManga(query, "")
		c.state.App.QueueUpdateDraw(func() {
			if err != nil {
				c.state.SetStatus(fmt.Sprintf("[red]Fetch catalog failed: %v", err))
				c.state.DetailView.SetText(fmt.Sprintf("\n[red]ERROR: Could not contact Catalog Service.\nDetails: %v\n\n[yellow]If Catalog Service (:9092) is stopped, Gin returns HTTP 503.", err))
				return
			}

			c.state.MangaCatalog = mangas
			table := c.state.CatalogTable
			for i := table.GetRowCount() - 1; i > 0; i-- {
				table.RemoveRow(i)
			}

			for idx, m := range mangas {
				row := idx + 1
				genres := strings.Join(m.Genres, ", ")
				table.SetCellSimple(row, 0, m.ID)
				table.SetCellSimple(row, 1, m.Title)
				table.SetCellSimple(row, 2, m.Author)
				table.SetCellSimple(row, 3, genres)
				table.SetCellSimple(row, 4, fmt.Sprintf("%d", m.TotalChapters))
				table.SetCellSimple(row, 5, m.Status)
			}

			c.state.SetStatus(fmt.Sprintf("[green]Loaded %d manga items from gRPC Catalog[white]", len(mangas)))
			if len(mangas) > 0 {
				c.renderMangaDetails(mangas[0])
			}
		})
	}()
}

func (c *UIController) renderMangaDetails(m *models.Manga) {
	genres := strings.Join(m.Genres, ", ")
	detailText := fmt.Sprintf(
		"\n [yellow:b]%s[-:-:-]\n"+
			" [gray]ID:[white] %s\n"+
			" [gray]Author:[white] %s\n"+
			" [gray]Genres:[white] %s\n"+
			" [gray]Status:[white] %s\n"+
			" [gray]Total Chapters:[white] %d\n\n"+
			" [yellow]Description:[-]\n %s\n\n"+
			" ──────────────────────────────────────────\n"+
			" [green]Press [Enter] or 'a' to Add to My Library[white]",
		m.Title, m.ID, m.Author, genres, m.Status, m.TotalChapters, m.Description,
	)
	c.state.DetailView.SetText(detailText)
}

func (c *UIController) showAddToLibraryDialog(m *models.Manga) {
	if c.state.Username == "" {
		c.state.SetStatus("[red]Please login first (Press F10) to add to library[white]")
		return
	}

	modal := tview.NewModal().
		SetText(fmt.Sprintf("Add '%s' to your reading library?", m.Title)).
		AddButtons([]string{"Reading", "Plan to Read", "Completed", "Cancel"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			c.state.Pages.RemovePage("dialog")
			if buttonLabel == "Cancel" {
				return
			}
			status := strings.ToLower(strings.ReplaceAll(buttonLabel, " ", "_"))
			go func() {
				err := c.api.AddToLibrary(m.ID, status)
				c.state.App.QueueUpdateDraw(func() {
					if err != nil {
						c.state.SetStatus(fmt.Sprintf("[red]Add to library failed: %v", err))
					} else {
						c.state.SetStatus(fmt.Sprintf("[green]Added '%s' to library as %s[white]", m.Title, status))
					}
				})
			}()
		})

	c.state.Pages.AddPage("dialog", modal, true, true)
}

// ==========================================
// 3. MY LIBRARY SCREEN (HTTP + TCP SYNC)
// ==========================================
func (c *UIController) buildLibraryView() tview.Primitive {
	c.state.LibraryTable = tview.NewTable().
		SetBorders(true).
		SetSelectable(true, false)

	c.state.LibraryTable.SetCell(0, 0, tview.NewTableCell("Manga ID").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	c.state.LibraryTable.SetCell(0, 1, tview.NewTableCell("Title").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	c.state.LibraryTable.SetCell(0, 2, tview.NewTableCell("Current Chapter").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	c.state.LibraryTable.SetCell(0, 3, tview.NewTableCell("Status").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	c.state.LibraryTable.SetCell(0, 4, tview.NewTableCell("Version").SetTextColor(tcell.ColorYellow).SetSelectable(false))

	// When row selected: show update progress dialog
	c.state.LibraryTable.SetSelectedFunc(func(row, column int) {
		if row <= 0 || row > len(c.state.UserLibrary) {
			return
		}
		item := c.state.UserLibrary[row-1]
		c.showUpdateProgressDialog(item)
	})

	instructions := tview.NewTextView().
		SetDynamicColors(true).
		SetText(" [yellow]Tips:[white] Press [Enter] on a manga row to update chapter progress.\n Progress updates are persisted in SQLite and pushed to other same-user sessions via [green]TCP (port 9090)[white].")

	flex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(c.state.LibraryTable, 0, 1, true).
		AddItem(instructions, 2, 0, false)
	flex.SetBorder(true).SetTitle(" My Reading Library (HTTP REST + Real-time TCP Progress Sync) ")

	return flex
}

func (c *UIController) refreshLibrary() {
	if c.state.Username == "" {
		c.state.SetStatus("[red]Please login first (Press F10) to view personal library[white]")
		return
	}

	c.state.SetStatus("[yellow]Fetching personal library...[white]")
	go func() {
		items, err := c.api.FetchLibrary()
		c.state.App.QueueUpdateDraw(func() {
			if err != nil {
				c.state.SetStatus(fmt.Sprintf("[red]Fetch library failed: %v", err))
				return
			}
			c.state.UserLibrary = items
			table := c.state.LibraryTable
			for i := table.GetRowCount() - 1; i > 0; i-- {
				table.RemoveRow(i)
			}
			for idx, it := range items {
				row := idx + 1
				table.SetCellSimple(row, 0, it.MangaID)
				table.SetCellSimple(row, 1, it.Title)
				table.SetCellSimple(row, 2, fmt.Sprintf("%d", it.CurrentChapter))
				table.SetCellSimple(row, 3, it.Status)
				table.SetCellSimple(row, 4, fmt.Sprintf("v%d", it.Version))
			}
			c.state.SetStatus(fmt.Sprintf("[green]Library loaded: %d items[white]", len(items)))
		})
	}()
}

func (c *UIController) showUpdateProgressDialog(item LibraryItem) {
	form := tview.NewForm()
	form.SetBorder(true).SetTitle(fmt.Sprintf(" Update Reading Progress: %s ", item.Title))

	var newChapterStr = fmt.Sprintf("%d", item.CurrentChapter+1)
	var newStatus = item.Status

	form.AddInputField("Chapter Number", newChapterStr, 10, nil, func(text string) {
		newChapterStr = text
	})
	form.AddDropDown("Status", []string{"reading", "completed", "plan_to_read"}, 0, func(option string, optionIndex int) {
		newStatus = option
	})

	statusFeedback := tview.NewTextView().SetDynamicColors(true)

	form.AddButton("Save & Broadcast TCP", func() {
		ch, err := strconv.Atoi(strings.TrimSpace(newChapterStr))
		if err != nil || ch < 0 {
			statusFeedback.SetText("[red]Please enter a valid non-negative integer")
			return
		}

		go func() {
			err := c.api.UpdateProgress(item.MangaID, ch, newStatus)
			c.state.App.QueueUpdateDraw(func() {
				if err != nil {
					statusFeedback.SetText(fmt.Sprintf("[red]Update failed: %v", err))
					return
				}
				c.state.Pages.RemovePage("update_dialog")
				c.refreshLibrary()
				c.state.SetStatus(fmt.Sprintf("[green]Updated '%s' to chapter %d. TCP event broadcasted![white]", item.Title, ch))
			})
		}()
	})

	form.AddButton("Cancel", func() {
		c.state.Pages.RemovePage("update_dialog")
	})

	flex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(form, 9, 0, true).
		AddItem(statusFeedback, 2, 0, false)

	c.state.Pages.AddPage("update_dialog", centerPrimitive(flex, 54, 13), true, true)
}

// ==========================================
// 4. DISCUSSION SCREEN (WebSocket Chat)
// ==========================================
func (c *UIController) buildDiscussionView() tview.Primitive {
	// Room list selector
	roomList := tview.NewList().
		AddItem("#general", "Main community manga discussion", '1', nil).
		AddItem("#one-piece", "One Piece chapter discussions", '2', nil).
		AddItem("#recommendations", "Manga discovery and recommendations", '3', nil).
		AddItem("#spoilers", "Latest chapter release spoiler talks", '4', nil)
	roomList.SetBorder(true).SetTitle(" Rooms (Room Isolation) ")

	// Message history log
	c.state.ChatMessagesView = tview.NewTextView().
		SetDynamicColors(true).
		SetWordWrap(true).
		SetScrollable(true)
	c.state.ChatMessagesView.SetBorder(true).SetTitle(" Live Chat Feed (WebSocket) ")

	// Input bar
	msgInput := tview.NewInputField().
		SetLabel(" Message: ").
		SetFieldWidth(60)

	msgInput.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			text := strings.TrimSpace(msgInput.GetText())
			if text != "" {
				if err := c.chat.SendMessage(text); err != nil {
					c.state.SetStatus(fmt.Sprintf("[red]Chat send error: %v[white]", err))
				} else {
					msgInput.SetText("")
				}
			}
		}
	})

	roomList.SetSelectedFunc(func(idx int, mainText, secondaryText string, shortcut rune) {
		cleanRoom := strings.TrimPrefix(mainText, "#")
		_ = c.chat.Connect(cleanRoom)
		c.state.App.SetFocus(msgInput)
	})

	chatPane := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(c.state.ChatMessagesView, 0, 1, false).
		AddItem(msgInput, 1, 0, true)

	split := tview.NewFlex().
		AddItem(roomList, 28, 0, false).
		AddItem(chatPane, 0, 1, true)

	split.SetBorder(true).SetTitle(" Community Discussion (Gorilla WebSocket, Room Isolation) ")
	return split
}

// ==========================================
// 5. NOTIFICATIONS SCREEN (UDP Datagrams)
// ==========================================
func (c *UIController) buildNotificationsView() tview.Primitive {
	c.state.NotifView = tview.NewTextView().
		SetDynamicColors(true).
		SetWordWrap(true).
		SetText("\n [yellow]Listening for UDP chapter release datagrams on port 9091...[white]\n\n" +
			" When admin triggers POST /api/admin/manga/:id/release,\n" +
			" the backend dispatches a datagram event to all subscribed sessions.\n\n" +
			" [gray]No notifications received yet.[white]")
	c.state.NotifView.SetBorder(true).SetTitle(" UDP Chapter Announcements (Best-effort Datagrams) ")
	return c.state.NotifView
}

// ==========================================
// 6. HELP & SYSTEM STATUS SCREEN
// ==========================================
func (c *UIController) buildHelpView() tview.Primitive {
	helpText := tview.NewTextView().
		SetDynamicColors(true).
		SetText("\n" +
			" [yellow:b]========================================================================[-:-:-]\n" +
			" [green:b]           MANGAHUB NETWORK PROTOCOL INTEGRATION OVERVIEW (IT096IIU)     [-:-:-]\n" +
			" [yellow:b]========================================================================[-:-:-]\n\n" +
			" [white:blue:b] 1. HTTP REST (:8080) [-:-:-]\n" +
			"    • Authentication (Register, Login with JWT tokens, bcrypt password hashing).\n" +
			"    • Personal reading library CRUD and progress tracking.\n" +
			"    • Proper HTTP status codes: 200, 400 (bad input), 401 (unauthorized), 404, 409 (conflict), 503.\n\n" +
			" [white:blue:b] 2. gRPC Catalog Service (:9092) [-:-:-]\n" +
			"    • Independent service process holding SQLite catalog (manga table).\n" +
			"    • Gin calls Unary RPCs: SearchManga, GetManga, UpdateChapters with a 2-second client deadline.\n" +
			"    • [yellow]Demo Fault Injection: Stop Catalog Service; Gin returns bounded 503 instead of hanging.[-]\n\n" +
			" [white:blue:b] 3. TCP Progress Sync (:9090) [-:-:-]\n" +
			"    • Long-lived socket connection with newline framing (\\n).\n" +
			"    • [yellow]User Isolation: Progress updates are broadcast ONLY to other sessions of the SAME user.[-]\n" +
			"    • Session B updates automatically without manual refresh or HTTP polling.\n\n" +
			" [white:blue:b] 4. UDP Chapter Releases (:9091) [-:-:-]\n" +
			"    • Connectionless datagram notification system.\n" +
			"    • Admin triggers chapter release -> backend broadcasts UDP new_chapter datagrams.\n" +
			"    • Displays in-app notification banner.\n\n" +
			" [white:blue:b] 5. WebSocket Chat Room (:8080/ws) [-:-:-]\n" +
			"    • Full-duplex interactive live discussion among TUI clients.\n" +
			"    • [yellow]Room Isolation server-side: Messages in #general never leak into #one-piece.[-]\n\n" +
			" ────────────────────────────────────────────────────────────────────────\n" +
			" [gray]KEYBOARD SHORTCUTS: [yellow]F1[white]:Browser | [yellow]F2[white]:Library | [yellow]F3[white]:Chat | [yellow]F4[white]:Notif | [yellow]F5[white]:Help | [yellow]F10[white]:Account\n" +
			" [gray]Press [yellow]Alt+Q[gray] or [yellow]Ctrl+C[gray] to exit application.[-]")

	helpText.SetBorder(true).SetTitle(" System Architecture & Protocol Verification Guide ")
	return helpText
}

// Utility: Centers a primitive on screen
func centerPrimitive(p tview.Primitive, width, height int) tview.Primitive {
	return tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(p, height, 1, true).
			AddItem(nil, 0, 1, false), width, 1, true).
		AddItem(nil, 0, 1, false)
}

