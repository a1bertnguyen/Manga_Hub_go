package tui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// buildDiscussionView constructs the WebSocket live chat room with strict room isolation
func (c *UIController) buildDiscussionView() tview.Primitive {
	// 1. Rooms List Selector
	roomList := tview.NewList().
		AddItem("#general", "General Manga Discussion", '1', nil).
		AddItem("#one-piece", "One Piece Discussions", '2', nil).
		AddItem("#recommendations", "Manga Recommendations", '3', nil).
		AddItem("#spoilers", "Latest Chapter Spoilers", '4', nil)
	roomList.SetBorder(true).
		SetTitle(" 💬 Rooms (Isolation) ").
		SetTitleColor(tcell.ColorCadetBlue)

	// 2. Chat Feed View
	c.state.ChatMessagesView = tview.NewTextView().
		SetDynamicColors(true).
		SetWordWrap(true).
		SetScrollable(true)
	c.state.ChatMessagesView.SetBorder(true).
		SetTitle(" 💬 Live Discussion Feed (WebSocket :8080/ws) ").
		SetTitleColor(tcell.ColorCadetBlue)
	c.state.ChatMessagesView.SetText(fmt.Sprintf(
		"\n [yellow:b]=== Welcome to #%s Live Discussion ===[-:-:-]\n"+
			" [gray]Messages in this room are strictly isolated to users connected to #%s.[white]\n"+
			" [gray]Press [yellow]Tab[gray] to toggle between rooms list and input.[white]\n\n",
		c.state.ActiveRoom, c.state.ActiveRoom,
	))

	// 3. Message Input Bar
	senderLabel := " 💬 Message: "
	if c.state.Username != "" {
		senderLabel = fmt.Sprintf(" 💬 [%s] > ", c.state.Username)
	}

	msgInput := tview.NewInputField().
		SetLabel(senderLabel).
		SetFieldWidth(60).
		SetPlaceholder("Type a message and press Enter...")

	// Send message handler
	msgInput.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			text := strings.TrimSpace(msgInput.GetText())
			if text == "" {
				return
			}

			if c.state.Username == "" {
				c.state.SetStatus("[red]Please login first (Press 0 or F10) to post messages[white]")
				return
			}

			if err := c.chat.SendMessage(text); err != nil {
				c.state.SetStatus(fmt.Sprintf("[red]Chat send error: %v[white]", err))
			} else {
				msgInput.SetText("")
			}
		}
	})

	// Room change handler
	roomList.SetSelectedFunc(func(idx int, mainText, secondaryText string, shortcut rune) {
		cleanRoom := strings.TrimPrefix(mainText, "#")
		if c.state.ActiveRoom != cleanRoom {
			c.state.ActiveRoom = cleanRoom
			if c.state.Username != "" {
				_ = c.chat.Connect(cleanRoom)
			} else {
				c.state.ChatMessagesView.SetText(fmt.Sprintf(
					"\n [yellow:b]=== Switched to Room: #%s ===[-:-:-]\n"+
						" [gray]Please login (Press 0 or F10) to send and receive messages.[white]\n\n",
					cleanRoom,
				))
			}
		}
		c.state.App.SetFocus(msgInput)
	})

	// Focus navigation with Tab
	msgInput.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyTab {
			c.state.App.SetFocus(roomList)
			return nil
		}
		return event
	})

	roomList.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyTab {
			c.state.App.SetFocus(msgInput)
			return nil
		}
		return event
	})

	chatPane := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(c.state.ChatMessagesView, 0, 1, false).
		AddItem(msgInput, 1, 0, true)

	split := tview.NewFlex().
		AddItem(roomList, 28, 0, false).
		AddItem(chatPane, 0, 1, true)

	return split
}
