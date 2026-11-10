package tui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"time"
)

type TCPWorker struct {
	state *AppState
	stop  chan struct{}
}

func NewTCPWorker(state *AppState) *TCPWorker {
	return &TCPWorker{
		state: state,
		stop:  make(chan struct{}),
	}
}

func (w *TCPWorker) Start() {
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

				w.connectAndListen()
				time.Sleep(3 * time.Second) // backoff before reconnect
			}
		}
	}()
}

func (w *TCPWorker) Stop() {
	close(w.stop)
}

func (w *TCPWorker) connectAndListen() {
	conn, err := net.DialTimeout("tcp", w.state.TCPAddr, 3*time.Second)
	if err != nil {
		w.state.TCPConnected = false
		w.state.SetStatus("[red]TCP sync disconnected (retrying...)[white]")
		return
	}
	defer conn.Close()

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
	w.state.SetStatus("[green]TCP sync connected[white]")

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var event struct {
			Type        string `json:"type"`
			UserID      string `json:"user_id"`
			MangaID     string `json:"manga_id"`
			MangaTitle  string `json:"manga_title"`
			Chapter     int    `json:"chapter"`
			Status      string `json:"status"`
			Version     int    `json:"version"`
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

			// Dispatch UI redraw without refresh
			w.state.App.QueueUpdateDraw(func() {
				if w.state.LibraryTable != nil {
					// Re-render library table rows
					w.refreshLibraryTableRows()
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
}

func (w *TCPWorker) refreshLibraryTableRows() {
	table := w.state.LibraryTable
	if table == nil {
		return
	}

	w.state.mu.RLock()
	defer w.state.mu.RUnlock()

	// Clear rows except header (row 0)
	for i := table.GetRowCount() - 1; i > 0; i-- {
		table.RemoveRow(i)
	}

	for idx, item := range w.state.UserLibrary {
		row := idx + 1
		table.SetCellSimple(row, 0, item.MangaID)
		table.SetCellSimple(row, 1, item.Title)
		table.SetCellSimple(row, 2, fmt.Sprintf("%d", item.CurrentChapter))
		table.SetCellSimple(row, 3, item.Status)
		table.SetCellSimple(row, 4, fmt.Sprintf("v%d", item.Version))
	}
	log.Printf("[TUI TCP] Redrew %d library rows from background TCP event", len(w.state.UserLibrary))
}

