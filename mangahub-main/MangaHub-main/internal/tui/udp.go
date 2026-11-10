package tui

import (
	"encoding/json"
	"fmt"
	"net"
	"time"
)

type UDPWorker struct {
	state *AppState
	stop  chan struct{}
}

func NewUDPWorker(state *AppState) *UDPWorker {
	return &UDPWorker{
		state: state,
		stop:  make(chan struct{}),
	}
}

func (w *UDPWorker) Start() {
	go func() {
		for {
			select {
			case <-w.stop:
				return
			default:
				if w.state.UserID == "" {
					time.Sleep(1 * time.Second)
					continue
				}

				w.listenAndSubscribe()
				time.Sleep(3 * time.Second)
			}
		}
	}()
}

func (w *UDPWorker) Stop() {
	close(w.stop)
}

func (w *UDPWorker) listenAndSubscribe() {
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
	defer conn.Close()

	// 1. Send subscription datagram
	subMsg := []byte("{\"type\":\"subscribe\"}\n")
	_, err = conn.WriteToUDP(subMsg, serverAddr)
	if err != nil {
		w.state.UDPSubscribed = false
		return
	}

	w.state.UDPSubscribed = true
	w.state.SetStatus("[green]UDP alerts subscribed[white]")

	// Periodic keepalive to avoid server timeout
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	go func() {
		for {
			select {
			case <-w.stop:
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

			notifText := fmt.Sprintf("🔔 NEW CHAPTER: '%s' Chapter %d is now available!", title, event.Chapter)
			w.state.AddNotification(notifText)
		}
	}

	w.state.UDPSubscribed = false
}

