package tui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// buildAuthView returns either the user profile screen (if logged in) or the login/register form
func (c *UIController) buildAuthView() tview.Primitive {
	if c.state.Username != "" {
		return c.buildProfileView()
	}
	return c.buildLoginForm()
}

func (c *UIController) buildProfileView() tview.Primitive {
	card := tview.NewFlex().SetDirection(tview.FlexRow)
	card.SetBorder(true).
		SetTitle(fmt.Sprintf(" 👤 User Profile: %s ", c.state.Username)).
		SetTitleColor(tcell.ColorYellow)

	tokenPreview := c.state.Token
	if len(tokenPreview) > 32 {
		tokenPreview = tokenPreview[:24] + "..." + tokenPreview[len(tokenPreview)-8:]
	}

	tcpStatus := "[red]Disconnected[white]"
	if c.state.TCPConnected {
		tcpStatus = "[green]Connected (:9090)[white]"
	}

	udpStatus := "[red]Unsubscribed[white]"
	if c.state.UDPSubscribed {
		udpStatus = "[green]Subscribed (:9091)[white]"
	}

	wsStatus := "[red]Disconnected[white]"
	if c.state.WSConnected {
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
			c.state.Username, c.state.UserID, tokenPreview,
			tcpStatus, udpStatus, wsStatus,
		))

	form := tview.NewForm()
	form.AddButton("Logout", func() {
		c.onLogout()
	})
	form.AddButton("My Library (2)", func() {
		c.SwitchTab("library")
	})
	form.AddButton("Manga Catalog (1)", func() {
		c.SwitchTab("browser")
	})

	card.AddItem(infoText, 0, 1, false).
		AddItem(form, 3, 0, true)

	return centerPrimitive(card, 64, 16)
}

func (c *UIController) buildLoginForm() tview.Primitive {
	form := tview.NewForm()
	form.SetBorder(true).
		SetTitle(" 🔐 MangaHub Account Authentication ").
		SetTitleColor(tcell.ColorYellow)

	var username, email, password string
	form.AddInputField("Username", "", 24, nil, func(text string) { username = strings.TrimSpace(text) })
	form.AddPasswordField("Password", "", 24, '*', func(text string) { password = text })
	form.AddInputField("Email (Register only)", "", 24, nil, func(text string) { email = strings.TrimSpace(text) })

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

	form.AddButton("Browse as Guest", func() {
		c.SwitchTab("browser")
	})

	box := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(form, 12, 0, true).
		AddItem(statusText, 2, 0, false)

	return centerPrimitive(box, 64, 16)
}

func (c *UIController) onLoginSuccess() {
	c.state.SetStatus(fmt.Sprintf("[green]Welcome, %s! Starting background TCP/UDP sync services...[white]", c.state.Username))

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

	c.state.Token = ""
	c.state.UserID = ""
	c.state.Username = ""
	c.state.UserLibrary = make([]LibraryItem, 0)

	// Rebuild auth page to display login form
	c.state.Pages.AddPage("auth", c.buildAuthView(), true, true)
	c.SwitchTab("auth")
	c.state.SetStatus("[yellow]Logged out successfully. Browsing as Guest.[white]")
}
