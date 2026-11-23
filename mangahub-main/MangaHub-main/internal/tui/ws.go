package tui

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"mangahub/pkg/models"

	"github.com/gorilla/websocket"
	"github.com/rivo/tview"
)

type ChatClient struct {
	state   *AppState
	conn    *websocket.Conn
	mu      sync.Mutex
	pending map[string]pendingChat
}

type pendingChat struct {
	conn   *websocket.Conn
	result chan error
}

func NewChatClient(state *AppState) *ChatClient {
	return &ChatClient{
		state:   state,
		pending: make(map[string]pendingChat),
	}
}

func (c *ChatClient) Connect(room string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		c.failPendingLocked(c.conn, fmt.Errorf("room changed before server confirmed the message"))
		_ = c.conn.Close()
		c.conn = nil
	}

	u, err := url.Parse(c.state.WSURL)
	if err != nil {
		return err
	}

	q := u.Query()
	q.Set("room", room)
	u.RawQuery = q.Encode()

	dialer := *websocket.DefaultDialer
	dialer.HandshakeTimeout = 3 * time.Second
	dialer.NetDialContext = (&net.Dialer{Timeout: 3 * time.Second}).DialContext
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+c.state.token())
	conn, response, err := dialer.Dial(u.String(), headers)
	if err != nil {
		c.state.setConnection("ws", false)
		detail := "backend unavailable"
		if response != nil {
			detail = fmt.Sprintf("HTTP %d; sign in again if your session expired", response.StatusCode)
		}
		c.state.SetStatus(fmt.Sprintf("[red]Cannot join #%s: %s[-]", tview.Escape(room), detail))
		return err
	}

	c.conn = conn
	c.state.setConnection("ws", true)
	c.state.SetStatus(fmt.Sprintf("[green]Connected to chat room #%s[white]", room))

	// Clear previous chat view and display room header
	if c.state.ChatMessagesView != nil {
		// Do not wait for the event loop while holding c.mu: logout and shutdown
		// call Close from that loop and need the same mutex.
		go func() {
			c.state.App.QueueUpdateDraw(func() {
				c.mu.Lock()
				current := c.conn == conn
				c.mu.Unlock()
				if current {
					c.state.ChatMessagesView.SetTitle(" #" + tview.Escape(room) + " ")
				}
			})
		}()
	}

	go c.readPump(conn, room)
	return nil
}

func (c *ChatClient) readPump(conn *websocket.Conn, room string) {
	disconnectError := fmt.Errorf("chat disconnected before server confirmed the message")
	defer func() {
		c.mu.Lock()
		c.failPendingLocked(conn, disconnectError)
		current := c.conn == conn
		if c.conn == conn {
			c.conn = nil
			c.state.setConnection("ws", false)
		}
		c.mu.Unlock()
		_ = conn.Close()
		if current {
			c.state.SetStatus("[red]Chat disconnected. Reopen Discussion to reconnect; unsent text is kept.[-]")
		}
	}()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			disconnectError = fmt.Errorf("chat delivery not confirmed: %w", err)
			break
		}

		var chatMsg struct {
			ID       string `json:"id"`
			Type     string `json:"type"`
			Username string `json:"username"`
			Text     string `json:"text"`
			Time     string `json:"time"`
			Room     string `json:"room"`
		}

		if err := json.Unmarshal(message, &chatMsg); err != nil {
			continue
		}
		if chatMsg.Room != room {
			continue
		}

		timestamp := chatMsg.Time
		if timestamp == "" {
			timestamp = time.Now().Format("15:04")
		}

		var formatted string
		if chatMsg.Type == "system" {
			formatted = fmt.Sprintf("[gray]%s * %s *[-]\n", tview.Escape("["+timestamp+"]"), tview.Escape(chatMsg.Text))
		} else {
			authorColor := "cyan"
			if chatMsg.Username == c.state.username() {
				authorColor = "green"
			}
			formatted = fmt.Sprintf("%s [%s]%s:[white] %s[-]\n", tview.Escape("["+timestamp+"]"), authorColor, tview.Escape(chatMsg.Username), tview.Escape(chatMsg.Text))
		}

		c.state.App.QueueUpdateDraw(func() {
			c.mu.Lock()
			defer c.mu.Unlock()
			if c.conn != conn {
				return // Ignore a queued frame from a room that has since been left.
			}
			if c.state.ChatMessagesView != nil {
				c.state.ChatMessagesView.Write([]byte(formatted))
				c.state.ChatMessagesView.ScrollToEnd()
			}
			if chatMsg.Type == "chat" && chatMsg.Username == c.state.username() {
				if pending, ok := c.pending[chatMsg.ID]; ok && pending.conn == conn {
					delete(c.pending, chatMsg.ID)
					pending.result <- nil
				}
			}
		})
	}
}

func (c *ChatClient) SendMessage(text string) error {
	c.mu.Lock()

	if c.conn == nil {
		c.mu.Unlock()
		return fmt.Errorf("not connected to chat")
	}

	msg := map[string]string{
		"id":   rand.Text(),
		"type": "chat",
		"text": text,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		c.mu.Unlock()
		return err
	}
	if len(data) > models.MaxChatFrameBytes {
		c.mu.Unlock()
		return fmt.Errorf("message exceeds the %d-byte limit; shorten it and try again", models.MaxChatFrameBytes)
	}
	conn := c.conn
	result := make(chan error, 1)
	id := msg["id"]
	c.pending[id] = pendingChat{conn: conn, result: result}

	err = conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	if err == nil {
		err = conn.WriteMessage(websocket.TextMessage, data)
	}
	if err != nil {
		delete(c.pending, id)
	}
	c.mu.Unlock()
	if err != nil {
		return err
	}
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case err := <-result:
		return err
	case <-timer.C:
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return fmt.Errorf("server did not confirm the message; text kept for retry")
	}
}

// failPendingLocked releases senders when their connection closes or changes rooms.
func (c *ChatClient) failPendingLocked(conn *websocket.Conn, err error) {
	for id, pending := range c.pending {
		if pending.conn == conn {
			delete(c.pending, id)
			pending.result <- err
		}
	}
}

func (c *ChatClient) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		c.failPendingLocked(c.conn, fmt.Errorf("chat closed before server confirmed the message"))
		_ = c.conn.Close()
		c.conn = nil
		c.state.setConnection("ws", false)
	}
}
