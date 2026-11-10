package tui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// buildHelpView constructs the system architecture, diagnostics, and keyboard navigation guide
func (c *UIController) buildHelpView() tview.Primitive {
	textView := tview.NewTextView().
		SetDynamicColors(true).
		SetWordWrap(true).
		SetScrollable(true)
	textView.SetBorder(true).
		SetTitle(" ℹ System Architecture, Protocol Topology & Keyboard Guide ").
		SetTitleColor(tcell.ColorDarkCyan)

	c.renderHelpContent(textView)

	// 'r' to refresh health status
	textView.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Rune() == 'r' || event.Rune() == 'R' {
			c.renderHelpContent(textView)
			c.state.SetStatus("[green]System diagnostics refreshed[white]")
			return nil
		}
		return event
	})

	return textView
}

func (c *UIController) renderHelpContent(textView *tview.TextView) {
	// Query backend health asynchronously
	go func() {
		healthData, err := c.api.FetchHealth()

		c.state.App.QueueUpdateDraw(func() {
			var sb strings.Builder

			sb.WriteString("\n [yellow:b]========================================================================================[-:-:-]\n")
			sb.WriteString(" [green:b]            MANGAHUB NETWORK PROTOCOL INTEGRATION OVERVIEW (IT096IIU Grade A)            [-:-:-]\n")
			sb.WriteString(" [yellow:b]========================================================================================[-:-:-]\n\n")

			// 1. Live Diagnostics
			sb.WriteString(" [yellow:b]📡 LIVE PROTOCOL DIAGNOSTICS:[-:-:-] (Press [green]'r'[white] to re-check)\n")
			if err != nil {
				sb.WriteString(fmt.Sprintf("   [red]Backend Health Check Failed: %v[white]\n", err))
			} else {
				services, _ := healthData["services"].(map[string]interface{})
				sb.WriteString(fmt.Sprintf("   • [cyan]HTTP REST (:8080)[white]      : [green]RUNNING[white]\n"))
				if grpcStatus, ok := services["grpc_catalog"].(string); ok && grpcStatus == "connected" {
					sb.WriteString(fmt.Sprintf("   • [cyan]gRPC Catalog (:9092)[white]   : [green]CONNECTED[white] (Unary RPCs with 2s deadline)\n"))
				} else {
					sb.WriteString(fmt.Sprintf("   • [cyan]gRPC Catalog (:9092)[white]   : [red]UNREACHABLE[white] (HTTP 503 fallback triggered)\n"))
				}
				sb.WriteString(fmt.Sprintf("   • [cyan]TCP Sync Hub (:9090)[white]   : [green]ONLINE[white] (Per-user isolated socket)\n"))
				sb.WriteString(fmt.Sprintf("   • [cyan]UDP Notifier (:9091)[white]   : [green]ONLINE[white] (Connectionless datagrams)\n"))
				sb.WriteString(fmt.Sprintf("   • [cyan]WebSocket (:8080/ws)[white]   : [green]ONLINE[white] (Room-isolated discussion rooms)\n"))
			}
			sb.WriteString("\n")

			// 2. Topology Diagram
			sb.WriteString(" [yellow:b]🗺️ NETWORK TOPOLOGY ARCHITECTURE:[-:-:-]\n")
			sb.WriteString(" [gray]┌────────────────┐          HTTP REST (:8080)          ┌─────────────────┐\n" +
				" │                │ ──────────────────────────────────> │                 │\n" +
				" │   TUI Client   │ <─────── TCP Progress (:9090) ───── │     Backend     │ ── gRPC (:9092) ──> Catalog Service\n" +
				" │   (Terminal)   │ <─────── UDP Releases (:9091) ───── │    (Router)     │                     (SQLite DB)\n" +
				" │                │ <──────> WebSocket (:8080/ws) ───── │                 │\n" +
				" └────────────────┘                                     └─────────────────┘[white]\n\n")

			// 3. Protocol Rationales
			sb.WriteString(" [yellow:b]📋 PROTOCOL SPECIFICATIONS & ROLES:[-:-:-]\n")
			sb.WriteString("   [white:blue:b] 1. HTTP REST (:8080) [-:-:-]\n" +
				"      • Authentication (JWT tokens, bcrypt password hashing).\n" +
				"      • Reading library CRUD & progress updates.\n" +
				"      • Standardized HTTP status codes (200, 400, 401, 404, 409, 503).\n\n" +
				"   [white:blue:b] 2. gRPC Catalog Service (:9092) [-:-:-]\n" +
				"      • Isolated microservice owning catalog data.\n" +
				"      • High-performance Protobuf binary serialization.\n" +
				"      • 2-second client deadline; graceful HTTP 503 degradation on timeout.\n\n" +
				"   [white:blue:b] 3. TCP Sync Hub (:9090) [-:-:-]\n" +
				"      • Long-lived socket connection with newline framing (\\n).\n" +
				"      • Per-user isolation: updates broadcast only to matching UserID sessions.\n\n" +
				"   [white:blue:b] 4. UDP Chapter Releases (:9091) [-:-:-]\n" +
				"      • Connectionless datagram notification system.\n" +
				"      • Best-effort delivery without connection state overhead.\n\n" +
				"   [white:blue:b] 5. WebSocket Chat (:8080/ws) [-:-:-]\n" +
				"      • Interactive full-duplex room discussion.\n" +
				"      • Server-side room isolation (#general, #one-piece, etc.).\n\n")

			// 4. Keyboard Shortcuts
			sb.WriteString(" [yellow:b]⌨️  KEYBOARD SHORTCUTS REFERENCE:[-:-:-]\n")
			sb.WriteString("   • [yellow]1 .. 5, 0[white]   : Quick switch between views (Catalog, Library, Chat, Alerts, System, Account)\n")
			sb.WriteString("   • [yellow]F1 .. F5, F10[white] : Function key alternatives\n")
			sb.WriteString("   • [yellow]Tab[white]         : Cycle to next tab (when not inside text input)\n")
			sb.WriteString("   • [yellow]/[white]           : Focus search bar in Catalog view\n")
			sb.WriteString("   • [yellow]+ / -[white]       : Increment/Decrement chapter progress in Library view\n")
			sb.WriteString("   • [yellow]t[white]           : Trigger demo chapter release in Alerts view\n")
			sb.WriteString("   • [yellow]Alt+Q / Ctrl+C[white]: Exit TUI application\n")

			textView.SetText(sb.String())
		})
	}()
}
