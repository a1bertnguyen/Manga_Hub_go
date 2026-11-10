package tui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// buildNotificationsView constructs the UDP chapter release announcement screen
func (c *UIController) buildNotificationsView() tview.Primitive {
	c.state.NotifView = tview.NewTextView().
		SetDynamicColors(true).
		SetWordWrap(true).
		SetScrollable(true)
	c.state.NotifView.SetBorder(true).
		SetTitle(" 🔔 UDP Chapter Announcements (Port 9091 Best-Effort Datagrams) ").
		SetTitleColor(tcell.ColorOrange)

	c.renderNotificationsList()

	controlBar := tview.NewTextView().
		SetDynamicColors(true).
		SetText(" [yellow:b]ACTIONS:[-:-:-] [green][t] Trigger Demo Release (gRPC -> UDP)[white]  │  [yellow][c] Clear Alerts[white]  │  [cyan][r] Re-subscribe UDP[white]")

	flex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(c.state.NotifView, 0, 1, true).
		AddItem(controlBar, 1, 0, false)

	// Key handling for actions: 't' to trigger release, 'c' to clear, 'r' to re-subscribe
	flex.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case 't', 'T':
			c.triggerDemoChapterRelease()
			return nil
		case 'c', 'C':
			c.clearNotifications()
			return nil
		case 'r', 'R':
			c.udp.Start()
			c.state.SetStatus("[green]Re-subscribed to UDP chapter alerts on port 9091[white]")
			return nil
		}
		return event
	})

	return flex
}

func (c *UIController) renderNotificationsList() {
	if c.state.NotifView == nil {
		return
	}

	c.state.mu.RLock()
	count := len(c.state.NotificationList)
	listCopy := make([]string, count)
	copy(listCopy, c.state.NotificationList)
	c.state.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString("\n [yellow:b]=== UDP CHAPTER RELEASE ANNOUNCEMENT FEED ===[-:-:-]\n")
	sb.WriteString(" [gray]Backend dispatches UDP datagrams to port 9091 when new chapters are published.[white]\n")
	sb.WriteString(fmt.Sprintf(" [gray]Total notifications received:[white] [yellow]%d[white]\n\n", count))

	if count == 0 {
		sb.WriteString("  [gray]No notifications received yet.[white]\n\n")
		sb.WriteString("  [yellow]💡 PRO-TIP:[white] Press [green]'t'[white] right now to trigger a demo chapter release!\n")
		sb.WriteString("  The backend will call the gRPC Catalog Service, update total chapters, and\n")
		sb.WriteString("  broadcast a UDP datagram to all clients. You will see the event appear here instantly!\n")
	} else {
		for _, n := range listCopy {
			sb.WriteString(fmt.Sprintf("  [yellow]⚡[white] %s\n  [gray]────────────────────────────────────────────────────────────────────────[white]\n", n))
		}
	}

	c.state.NotifView.SetText(sb.String())
}

func (c *UIController) clearNotifications() {
	c.state.mu.Lock()
	c.state.NotificationList = make([]string, 0)
	c.state.mu.Unlock()

	c.renderNotificationsList()
	c.state.SetStatus("[yellow]Notifications cleared[white]")
}

func (c *UIController) triggerDemoChapterRelease() {
	mangaID := "manga_001"
	title := "One Piece"
	targetChapter := 1115

	c.state.SetStatus(fmt.Sprintf("[yellow]Triggering demo release for %s (Ch. %d)...[white]", title, targetChapter))

	go func() {
		err := c.api.TriggerRelease(mangaID, targetChapter, title)
		c.state.App.QueueUpdateDraw(func() {
			if err != nil {
				c.state.SetStatus(fmt.Sprintf("[red]Release failed: %v[white]", err))
				return
			}
			c.state.SetStatus(fmt.Sprintf("[green]Demo release sent! Waiting for UDP datagram on :9091...[white]"))
		})
	}()
}
