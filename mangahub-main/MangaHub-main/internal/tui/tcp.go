package tui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

type TCPWorker struct {
	state   *AppState
	stop    chan struct{}
	running bool
	conn    net.Conn
	mu      sync.Mutex
}

func NewTCPWorker(state *AppState) *TCPWorker {
	return &TCPWorker{
		state: state,
	}
}

func (w *TCPWorker) Start() {
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

				w.connectAndListen(stopChan)

				select {
				case <-stopChan:
					return
				case <-time.After(3 * time.Second): // backoff before reconnect
				}
			}
		}
	}()
}

func (w *TCPWorker) Stop() {
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
	w.state.TCPConnected = false
}

func (w *TCPWorker) connectAndListen(stopChan chan struct{}) {
	conn, err := net.DialTimeout("tcp", w.state.TCPAddr, 3*time.Second)
	if err != nil {
		w.state.TCPConnected = false
		w.state.SetStatus("[red]TCP sync disconnected (retrying...)[white]")
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

	// 1. Send first-message authentication
	authPayload := map[string]string{
		"type":    "auth",
		"token":   w.state.Token,
		"user_id": w.state.UserID,
	}
	authBytes, _ := json.Marshal(authPayload)
	_, err = conn.Write(append(authBytes, '\n'))
	if err != nil {
		w.state.TCPConnected = false
		return
	}

	w.state.TCPConnected = true
	w.state.SetStatus("[green]TCP sync connected (per-user isolated)[white]")

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var event struct {
			Type       string `json:"type"`
			UserID     string `json:"user_id"`
			MangaID    string `json:"manga_id"`
			MangaTitle string `json:"manga_title"`
			Chapter    int    `json:"chapter"`
			Status     string `json:"status"`
			Version    int    `json:"version"`
		}

		if err := json.Unmarshal(line, &event); err != nil {
			continue
		}

		if event.Type == "progress_updated" {
			title := event.MangaTitle
			if title == "" {
				title = event.MangaID
			}

			// Update in-memory user library
			w.state.mu.Lock()
			found := false
			for idx := range w.state.UserLibrary {
				if w.state.UserLibrary[idx].MangaID == event.MangaID {
					w.state.UserLibrary[idx].CurrentChapter = event.Chapter
					w.state.UserLibrary[idx].Version = event.Version
					if event.Status != "" {
						w.state.UserLibrary[idx].Status = event.Status
					}
					found = true
					break
				}
			}
			if !found {
				w.state.UserLibrary = append(w.state.UserLibrary, LibraryItem{
					MangaID:        event.MangaID,
					Title:          title,
					CurrentChapter: event.Chapter,
					Status:         event.Status,
					Version:        event.Version,
				})
			}
			w.state.mu.Unlock()

			// Dispatch UI redraw
			w.state.App.QueueUpdateDraw(func() {
				if w.state.LibraryTable != nil {
					w.state.mu.RLock()
					items := make([]LibraryItem, len(w.state.UserLibrary))
					copy(items, w.state.UserLibrary)
					w.state.mu.RUnlock()
					populateLibraryTable(w.state.LibraryTable, items, w.state.MangaCatalog)
				}
				w.state.SetStatus(fmt.Sprintf(
					"[green]⚡ [TCP Sync][white] %s -> Chapter %d (v%d)",
					title, event.Chapter, event.Version,
				))
			})
		}
	}

	w.state.TCPConnected = false
	w.state.SetStatus("[red]TCP sync connection lost[white]")
	log.Printf("[TUI TCP] Disconnected from %s", w.state.TCPAddr)
}
