package tui

import (
	"fmt"
	"strings"

	"github.com/rivo/tview"
)

// buildAuthView returns either the user profile screen (if logged in) or the login/register form
func (c *UIController) buildAuthView() tview.Primitive {
	if c.state.username() != "" {
		return c.buildProfileView()
	}
	return c.buildLoginForm()
}

func (c *UIController) buildProfileView() tview.Primitive {
	card := tview.NewFlex().SetDirection(tview.FlexRow)
	card.SetBorder(true).
		SetTitle(" Your account ").
		SetTitleColor(colorAccent)

	tokenPreview := "Saved on this device"

	tcpStatus := "[red]Disconnected[white]"
	if c.state.connected("tcp") {
		tcpStatus = "[green]Connected (:9090)[white]"
	}

	udpStatus := "[red]Unsubscribed[white]"
	if c.state.connected("udp") {
		udpStatus = "[green]Subscribed (:9091)[white]"
	}

	wsStatus := "[red]Disconnected[white]"
	if c.state.connected("ws") {
		wsStatus = fmt.Sprintf("[green]Connected (#%s)[white]", c.state.ActiveRoom)
	}

	infoText := tview.NewTextView().
		SetDynamicColors(true).
		SetText(fmt.Sprintf(
			"\n  [yellow:b]ACCOUNT DETAILS[-:-:-]\n"+
				"  • [gray]Username:[white]   [cyan:b]%s[-:-:-]\n"+
				"  • [gray]User ID:[white]    [white]%s\n"+
				"  • [gray]Auth Token:[white] [lightgray]%s\n\n"+
				"  [yellow:b]ACTIVE PROTOCOL SESSIONS[-:-:-]\n"+
				"  • [gray]TCP Progress Sync:[white] %s\n"+
				"  • [gray]UDP Chapter Alerts:[white] %s\n"+
				"  • [gray]WebSocket Chat:[white]    %s\n\n",
			c.state.username(), c.state.userID(), tokenPreview,
			tcpStatus, udpStatus, wsStatus,
		))

	form := tview.NewForm()
	styleForm(form)
	form.AddButton("Logout", func() {
		c.onLogout()
	})
	form.AddButton("My Library (2)", func() {
		c.SwitchTab("library")
	})
	form.AddButton("Manga Catalog (1)", func() {
		c.SwitchTab("browser")
	})

	c.focus["auth"] = []tview.Primitive{form}
	card.AddItem(infoText, 0, 1, false).
		AddItem(form, 3, 0, true)

	return centerPrimitive(card, 70, 22)
}

func (c *UIController) buildLoginForm() tview.Primitive {
	form := tview.NewForm()
	styleForm(form)
	form.SetBorder(true).
		SetTitle(" WELCOME TO MANGAHUB ").
		SetTitleColor(colorAccent)

	var username, email, password string
	form.AddInputField("Username", "", 24, nil, func(text string) { username = strings.TrimSpace(text) })
	form.AddPasswordField("Password", "", 24, '*', func(text string) { password = text })
	form.AddInputField("Email (new account)", "", 24, nil, func(text string) { email = strings.TrimSpace(text) })

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
			statusText.SetText("[red]Username, email, and password (min 8 chars) required")
			return
		}
		if len(password) < 8 {
			statusText.SetText("[red]Password must be at least 8 characters")
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

	form.AddButton("Catalog", func() {
		c.SwitchTab("browser")
	})

	c.focus["auth"] = []tview.Primitive{form}
	box := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(hint("Track chapters. Discover stories. Read together."), 2, 0, false).
		AddItem(form, 13, 0, true).
		AddItem(statusText, 2, 0, false)

	return centerPrimitive(box, 64, 19)
}

func (c *UIController) onLoginSuccess() {
	c.state.SetStatus(fmt.Sprintf("[green]Welcome, %s! Starting background TCP/UDP sync services...[white]", c.state.username()))

	// Rebuild auth page so it now displays the Profile View
	c.state.Pages.AddPage("auth", c.buildAuthView(), true, false)

	// Start background workers
	c.tcp.Start()
	c.udp.Start()

	// Switch to catalog or library
	c.SwitchTab("browser")
}

func (c *UIController) onLogout() {
	_ = ClearSession()

	c.tcp.Stop()
	c.udp.Stop()
	c.chat.Close()

	c.state.setSession("", "", "")
	c.state.mu.Lock()
	c.state.UserLibrary = make([]LibraryItem, 0)
	c.state.mu.Unlock()
	c.catalogRequest++
	c.detailRequest++
	c.state.MangaCatalog = nil
	populateLibraryTable(c.state.LibraryTable, nil, nil)
	c.state.CatalogTable.Clear()
	c.setupCatalogTableHeader()
	c.state.ChatMessagesView.Clear()

	// Rebuild auth page to display login form
	c.state.Pages.AddPage("auth", c.buildAuthView(), true, true)
	c.SwitchTab("auth")
	c.state.SetStatus("[yellow]Logged out successfully. Browsing as Guest.[white]")
}
