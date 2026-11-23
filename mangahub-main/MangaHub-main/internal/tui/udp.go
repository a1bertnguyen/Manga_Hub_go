package tui

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"
)

type UDPWorker struct {
	state   *AppState
	stop    chan struct{}
	running bool
	conn    *net.UDPConn
	mu      sync.Mutex
}

func NewUDPWorker(state *AppState) *UDPWorker {
	return &UDPWorker{
		state: state,
	}
}

func (w *UDPWorker) Start() {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return
	}
	w.running = true
	w.stop = make(chan struct{})
	stopChan := w.stop
	w.mu.Unlock()

	go func() {
		for {
			select {
			case <-stopChan:
				return
			default:
				if w.state.userID() == "" {
					time.Sleep(1 * time.Second)
					continue
				}

				w.listenAndSubscribe(stopChan)

				select {
				case <-stopChan:
					return
				case <-time.After(3 * time.Second):
				}
			}
		}
	}()
}

func (w *UDPWorker) Stop() {
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return
	}
	w.running = false
	close(w.stop)
	if w.conn != nil {
		// Unregister the same source socket before closing it. Closing a UDP
		// socket alone does not tell the server that a subscriber has left.
		if server, err := net.ResolveUDPAddr("udp", w.state.UDPAddr); err == nil {
			_ = w.conn.SetWriteDeadline(time.Now().Add(250 * time.Millisecond))
			_, _ = w.conn.WriteToUDP([]byte("{\"type\":\"unsubscribe\"}\n"), server)
		}
		_ = w.conn.Close()
		w.conn = nil
	}
	w.state.setConnection("udp", false)
	w.mu.Unlock()
}

func (w *UDPWorker) listenAndSubscribe(stopChan chan struct{}) {
	serverAddr, err := net.ResolveUDPAddr("udp", w.state.UDPAddr)
	if err != nil {
		w.state.setConnection("udp", false)
		return
	}

	conn, err := net.ListenUDP("udp", nil)
	if err != nil {
		w.state.setConnection("udp", false)
		return
	}

	w.mu.Lock()
	select {
	case <-stopChan:
		w.mu.Unlock()
		_ = conn.Close()
		return
	default:
		w.conn = conn
	}
	w.mu.Unlock()

	defer func() {
		_ = conn.Close()
		w.mu.Lock()
		if w.conn == conn {
			w.conn = nil
			w.state.setConnection("udp", false)
		}
		w.mu.Unlock()
	}()

	// 1. Send subscription datagram
	subMsg := []byte("{\"type\":\"subscribe\"}\n")
	_, err = conn.WriteToUDP(subMsg, serverAddr)
	if err != nil {
		w.state.setConnection("udp", false)
		return
	}

	// A successful local write does not confirm server registration.
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	confirmed := false

	// Periodic keepalive to prevent backend timeout
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	done := make(chan struct{})
	defer close(done)

	go func() {
		for {
			select {
			case <-stopChan:
				return
			case <-done:
				return
			case <-ticker.C:
				w.mu.Lock()
				if w.conn == conn {
					_, _ = conn.WriteToUDP(subMsg, serverAddr)
				}
				w.mu.Unlock()
			}
		}
	}()

	buf := make([]byte, 4096)
	for {
		n, source, err := conn.ReadFromUDP(buf)
		if err != nil {
			break
		}
		if source.Port != serverAddr.Port || source.Zone != serverAddr.Zone || !source.IP.Equal(serverAddr.IP) {
			continue
		}

		var event struct {
			Type       string `json:"type"`
			Status     string `json:"status"`
			MangaID    string `json:"manga_id"`
			MangaTitle string `json:"manga_title"`
			Chapter    int    `json:"chapter"`
			Timestamp  int64  `json:"timestamp"`
		}

		if err := json.Unmarshal(buf[:n], &event); err != nil {
			continue
		}

		if event.Type == "subscribed" && event.Status == "ok" {
			_ = conn.SetReadDeadline(time.Now().Add(45 * time.Second))
			w.mu.Lock()
			if w.conn != conn {
				w.mu.Unlock()
				return
			}
			w.state.setConnection("udp", true)
			w.mu.Unlock()
			if !confirmed {
				w.state.SetStatus("[green]UDP alert subscription confirmed[white]")
				confirmed = true
			}
			continue
		}
		if confirmed && event.Type == "new_chapter" {
			title := event.MangaTitle
			if title == "" {
				title = event.MangaID
			}

			notifText := fmt.Sprintf("⚡ NEW CHAPTER: '%s' Chapter %d is released!", title, event.Chapter)
			w.state.AddNotification(notifText)
		}
	}

	select {
	case <-stopChan:
	default:
		w.state.SetStatus("[yellow]UDP subscription unconfirmed. Retrying...[white]")
	}
}
