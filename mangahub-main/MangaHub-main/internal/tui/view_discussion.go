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
		AddItem("#general", "The community lounge", 0, nil).
		AddItem("#one-piece", "Grand Line conversations", 0, nil).
		AddItem("#recommendations", "Find your next read", 0, nil).
		AddItem("#spoilers", "Read at your own risk", 0, nil)
	stylePanel(roomList.Box, "ROOMS")
	roomList.SetMainTextColor(colorText).SetSecondaryTextColor(colorMuted).
		SetSelectedBackgroundColor(colorAccent).SetSelectedTextColor(colorBackground)

	// 2. Chat Feed View
	c.state.ChatMessagesView = tview.NewTextView().
		SetDynamicColors(true).
		SetWordWrap(true).
		SetScrollable(true)
	stylePanel(c.state.ChatMessagesView.Box, "CONVERSATION")
	c.state.ChatMessagesView.SetText(fmt.Sprintf(
		"[#5eead4::b]Welcome to #%s[-:-:-]\n\n"+
			"[#8fa4bd]Join the conversation in #%s.\nSign in with F10 to send and receive messages.\nUse Tab to move between rooms, history and your message.[-]\n",
		c.state.ActiveRoom, c.state.ActiveRoom,
	))

	// 3. Message Input Bar
	senderLabel := " Message  "

	msgInput := tview.NewInputField().
		SetLabel(senderLabel).
		SetFieldWidth(0).
		SetPlaceholder("Type a message and press Enter...")
	msgInput.SetLabelColor(colorAccent).SetFieldBackgroundColor(colorField).SetFieldTextColor(colorText)
	msgInput.SetPlaceholderTextColor(colorMuted)

	// Send message handler
	msgInput.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			text := strings.TrimSpace(msgInput.GetText())
			if text == "" {
				return
			}

			if c.state.username() == "" {
				c.state.SetStatus("[red]Please login first (Press 0 or F10) to post messages[white]")
				return
			}

			go func() {
				err := c.chat.SendMessage(text)
				c.state.App.QueueUpdateDraw(func() {
					if err != nil {
						c.state.SetStatus(fmt.Sprintf("[red]Chat send error: %s[-]", tview.Escape(err.Error())))
					} else if msgInput.GetText() == text {
						msgInput.SetText("")
					}
				})
			}()
		}
	})

	// Room change handler
	roomList.SetSelectedFunc(func(idx int, mainText, secondaryText string, shortcut rune) {
		cleanRoom := strings.TrimPrefix(mainText, "#")
		c.state.ChatMessagesView.SetTitle(" #" + cleanRoom + " ")
		if c.state.ActiveRoom != cleanRoom {
			c.state.ActiveRoom = cleanRoom
			c.state.ChatMessagesView.SetText("[#8fa4bd]Joining #" + tview.Escape(cleanRoom) + "...[-]\n\n")
			if c.state.username() != "" {
				go func() {
					_ = c.chat.Connect(cleanRoom)
				}()
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
	c.focus["discussion"] = []tview.Primitive{msgInput, roomList, c.state.ChatMessagesView}

	split := tview.NewFlex().
		AddItem(roomList, 24, 0, false).
		AddItem(chatPane, 0, 1, true)

	return split
}
