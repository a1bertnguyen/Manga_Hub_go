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
				if w.state.UserID == "" {
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
		_ = w.conn.Close()
		w.conn = nil
	}
	w.mu.Unlock()
	w.state.UDPSubscribed = false
}

func (w *UDPWorker) listenAndSubscribe(stopChan chan struct{}) {
	serverAddr, err := net.ResolveUDPAddr("udp", w.state.UDPAddr)
	if err != nil {
		w.state.UDPSubscribed = false
		return
	}

	conn, err := net.ListenUDP("udp", nil)
	if err != nil {
		w.state.UDPSubscribed = false
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
		}
		w.mu.Unlock()
	}()

	// 1. Send subscription datagram
	subMsg := []byte("{\"type\":\"subscribe\"}\n")
	_, err = conn.WriteToUDP(subMsg, serverAddr)
	if err != nil {
		w.state.UDPSubscribed = false
		return
	}

	w.state.UDPSubscribed = true
	w.state.SetStatus("[green]UDP alerts subscribed (port 9091)[white]")

	// Periodic keepalive to prevent backend timeout
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	go func() {
		for {
			select {
			case <-stopChan:
				return
			case <-ticker.C:
				_, _ = conn.WriteToUDP([]byte("{\"type\":\"subscribe\"}\n"), serverAddr)
			}
		}
	}()

	buf := make([]byte, 4096)
	for {
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			break
		}

		var event struct {
			Type       string `json:"type"`
			MangaID    string `json:"manga_id"`
			MangaTitle string `json:"manga_title"`
			Chapter    int    `json:"chapter"`
			Timestamp  int64  `json:"timestamp"`
		}

		if err := json.Unmarshal(buf[:n], &event); err != nil {
			continue
		}

		if event.Type == "new_chapter" {
			title := event.MangaTitle
			if title == "" {
				title = event.MangaID
			}

			notifText := fmt.Sprintf("⚡ NEW CHAPTER: '%s' Chapter %d is released!", title, event.Chapter)
			w.state.AddNotification(notifText)
		}
	}

	w.state.UDPSubscribed = false
}
