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
	stylePanel(c.state.NotifView.Box, "CHAPTER ALERTS")

	c.renderNotificationsList()

	controlBar := tview.NewTextView().
		SetDynamicColors(true).
		SetText(" [#8fa4bd]s Subscribe   u Unsubscribe   t Demo release   c Clear[-]")

	flex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(c.state.NotifView, 0, 1, true).
		AddItem(controlBar, 1, 0, false)

	// Key handling for actions: 't' to trigger release, 'c' to clear, 'r' to re-subscribe
	c.state.NotifView.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case 't', 'T':
			c.triggerDemoChapterRelease()
			return nil
		case 'u', 'U':
			c.udp.Stop()
			c.state.SetStatus("Chapter alerts unsubscribed. Press s to subscribe again.")
			return nil
		case 'c', 'C':
			c.clearNotifications()
			return nil
		case 's', 'S', 'r', 'R':
			if c.state.username() == "" {
				c.state.SetStatus("Sign in with F10 to subscribe to chapter alerts.")
				return nil
			}
			c.udp.Start()
			c.state.SetStatus("Subscribing to chapter alerts...")
			return nil
		}
		return event
	})

	c.focus["notifications"] = []tview.Primitive{c.state.NotifView}
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
	sb.WriteString("[#5eead4::b]The latest from your reading world[-:-:-]\n")
	sb.WriteString(fmt.Sprintf("[#8fa4bd]%d recent alerts / best-effort delivery[-]\n\n", count))
	if count == 0 {
		sb.WriteString("No new chapters yet.\n\nSign in and press s to subscribe.\nFor a connected demo, press t to publish a chapter.\n\n[#8fa4bd]Alerts arrive here automatically while you browse.[-]")
	} else {
		for _, n := range listCopy {
			fmt.Fprintf(&sb, "[#5eead4]NEW CHAPTER[-]\n%s\n\n", tview.Escape(n))
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
	mangaID := "one-piece"
	title := "One Piece"
	targetChapter := 1105

	c.state.mu.RLock()
	if len(c.state.MangaCatalog) > 0 {
		m := c.state.MangaCatalog[0]
		mangaID = m.ID
		title = m.Title
		targetChapter = int(m.TotalChapters) + 1
	}
	c.state.mu.RUnlock()

	c.state.SetStatus(fmt.Sprintf("[yellow]Triggering demo release for %s (Ch. %d)...[white]", title, targetChapter))

	go func() {
		err := c.api.TriggerRelease(mangaID, targetChapter, title)
		if err != nil {
			c.state.SetStatus(fmt.Sprintf("[red]Release failed: %v[white]", err))
			return
		}
		c.state.SetStatus("[green]Demo release sent! Waiting for UDP datagram on :9091...[white]")
	}()
}
