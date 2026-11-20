package tui

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rivo/tview"
)

type ChatClient struct {
	state *AppState
	conn  *websocket.Conn
	mu    sync.Mutex
}

func NewChatClient(state *AppState) *ChatClient {
	return &ChatClient{
		state: state,
	}
}

func (c *ChatClient) Connect(room string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
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
	conn, _, err := dialer.Dial(u.String(), headers)
	if err != nil {
		c.state.setConnection("ws", false)
		c.state.SetStatus(fmt.Sprintf("[red]WebSocket failed to connect to room #%s[white]", room))
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
	defer func() {
		c.mu.Lock()
		if c.conn == conn {
			c.conn = nil
			c.state.setConnection("ws", false)
		}
		c.mu.Unlock()
		_ = conn.Close()
	}()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			break
		}

		var chatMsg struct {
			Type     string `json:"type"`
			Username string `json:"username"`
			Text     string `json:"text"`
			Time     string `json:"time"`
			Room     string `json:"room"`
		}

		if err := json.Unmarshal(message, &chatMsg); err != nil {
			continue
		}

		timestamp := chatMsg.Time
		if timestamp == "" {
			timestamp = time.Now().Format("15:04")
		}

		var formatted string
		if chatMsg.Type == "system" {
			formatted = fmt.Sprintf("[gray][%s] * %s *[white]\n", timestamp, chatMsg.Text)
		} else {
			authorColor := "cyan"
			if chatMsg.Username == c.state.username() {
				authorColor = "green"
			}
			formatted = fmt.Sprintf("[%s] [%s]%s:[white] %s\n", timestamp, authorColor, chatMsg.Username, chatMsg.Text)
		}

		c.state.App.QueueUpdateDraw(func() {
			if c.state.ChatMessagesView != nil {
				c.state.ChatMessagesView.Write([]byte(formatted))
				c.state.ChatMessagesView.ScrollToEnd()
			}
		})
	}
}

func (c *ChatClient) SendMessage(text string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return fmt.Errorf("not connected to chat")
	}

	msg := map[string]string{
		"type":     "chat",
		"username": c.state.username(),
		"text":     text,
		"room":     c.state.ActiveRoom,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	if err := c.conn.SetWriteDeadline(time.Now().Add(3 * time.Second)); err != nil {
		return err
	}
	return c.conn.WriteMessage(websocket.TextMessage, data)
}

func (c *ChatClient) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
		c.state.setConnection("ws", false)
	}
}
