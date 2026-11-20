package tui

import (
	"fmt"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"strings"
)

func (c *UIController) buildHelpView() tview.Primitive {
	view := tview.NewTextView().SetDynamicColors(true).SetWordWrap(true).SetScrollable(true)
	stylePanel(view.Box, "CONNECTIONS / HELP")
	view.SetText("Press r to check your connections.")
	view.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Rune() == 'r' || event.Rune() == 'R' {
			c.renderHelpContent(view)
			return nil
		}
		return event
	})
	c.focus["help"] = []tview.Primitive{view}
	return view
}

func (c *UIController) renderHelpContent(view *tview.TextView) {
	view.SetText("Checking backend and catalog...\n\nThe interface stays available while diagnostics run.")
	go func() {
		health, err := c.api.FetchHealth()
		c.state.App.QueueUpdateDraw(func() {
			var out strings.Builder
			out.WriteString("[#5eead4::b]CONNECTION STATUS[-:-:-]\n\n")
			if err != nil {
				fmt.Fprintf(&out, "[red]Backend unavailable[-]\n%s\n\n", tview.Escape(err.Error()))
			} else {
				out.WriteString("[green]Backend responded[-]\n")
				services, _ := health["services"].(map[string]interface{})
				fmt.Fprintf(&out, "Catalog: %s\n\n", tview.Escape(fmt.Sprint(services["grpc_catalog"])))
			}
			// Report actual client flags, rather than claiming every listener is online.
			c.state.mu.RLock()
			tcp, udp, ws := c.state.TCPConnected, c.state.UDPSubscribed, c.state.WSConnected
			c.state.mu.RUnlock()
			fmt.Fprintf(&out, "Progress sync (TCP): %s\nChapter alerts (UDP): %s\nDiscussion (WS): %s\n\n", connectionLabel(tcp), connectionLabel(udp), connectionLabel(ws))
			out.WriteString("[#5eead4::b]YOUR ENDPOINTS[-:-:-]\n\n")
			fmt.Fprintf(&out, "HTTP  %s\nTCP   %s\nUDP   %s\nWS    %s\n\n", tview.Escape(c.state.BaseURL), tview.Escape(c.state.TCPAddr), tview.Escape(c.state.UDPAddr), tview.Escape(c.state.WSURL))
			out.WriteString("[#5eead4::b]KEYBOARD GUIDE[-:-:-]\n\nF1 Catalog   F2 Library   F3 Discussion\nF4 Alerts    F5 Connections   F10 Account\n\nTab / Shift+Tab   Move between controls\nAlt+Left / Right Switch page\n/ Search catalog   g Change genre\nEnter Add a title, edit progress or send a message\n+ / - Change chapter in your library\ns / u Subscribe / unsubscribe from alerts\nt Demo chapter release   c Clear alerts\nr Refresh the current page   Ctrl+C / Alt+Q Quit\n\n[#5eead4::b]HOW IT CONNECTS[-:-:-]\n\nTUI -> HTTP / Gin -> gRPC catalog -> SQLite\nBackend -> TCP -> your other signed-in sessions\nBackend -> UDP -> subscribed chapter alerts\nTUI <-> WebSocket <-> discussion room\n\n[#8fa4bd]UDP alerts are best effort and may be lost. Refresh the catalog for current chapter counts. TCP reconnects after a dropped connection. Re-enter Discussion to reconnect chat.[-]")
			view.SetText(out.String())
		})
	}()
}

func connectionLabel(connected bool) string {
	if connected {
		return "[green]Active[-]"
	}
	return "[#8fa4bd]Inactive[-]"
}
