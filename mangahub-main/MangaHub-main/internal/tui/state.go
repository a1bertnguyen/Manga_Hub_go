package tui

import (
	"fmt"
	"sync"
	"time"

	"mangahub/pkg/models"

	"github.com/rivo/tview"
)

type LibraryItem struct {
	MangaID        string `json:"manga_id"`
	Title          string `json:"title"`
	CurrentChapter int    `json:"current_chapter"`
	Status         string `json:"status"`
	Version        int    `json:"version"`
}

type AppState struct {
	App   *tview.Application
	Pages *tview.Pages

	// Server URLs
	BaseURL  string
	TCPAddr  string
	UDPAddr  string
	WSURL    string

	// Auth Info
	Token    string
	UserID   string
	Username string

	// Live Data
	MangaCatalog     []*models.Manga
	UserLibrary      []LibraryItem
	NotificationList []string
	ActiveRoom       string

	// Connection status indicators
	TCPConnected  bool
	UDPSubscribed bool
	WSConnected   bool

	// Global UI Widgets
	StatusBar        *tview.TextView
	LibraryTable     *tview.Table
	CatalogTable     *tview.Table
	DetailView       *tview.TextView
	ChatMessagesView *tview.TextView
	NotifView        *tview.TextView

	mu sync.RWMutex
}

func NewAppState(app *tview.Application, pages *tview.Pages, baseURL, tcpAddr, udpAddr, wsURL string) *AppState {
	return &AppState{
		App:              app,
		Pages:            pages,
		BaseURL:          baseURL,
		TCPAddr:          tcpAddr,
		UDPAddr:          udpAddr,
		WSURL:            wsURL,
		ActiveRoom:       "general",
		NotificationList: make([]string, 0),
		MangaCatalog:     make([]*models.Manga, 0),
		UserLibrary:      make([]LibraryItem, 0),
	}
}

func (s *AppState) SetStatus(msg string) {
	if s.StatusBar == nil {
		return
	}
	s.App.QueueUpdateDraw(func() {
		userDisplay := "Guest"
		if s.Username != "" {
			userDisplay = s.Username
		}

		tcpDot := "[red]●[white]"
		if s.TCPConnected {
			tcpDot = "[green]●[white]"
		}

		udpDot := "[red]●[white]"
		if s.UDPSubscribed {
			udpDot = "[green]●[white]"
		}

		wsDot := "[red]●[white]"
		if s.WSConnected {
			wsDot = "[green]●[white]"
		}

		s.StatusBar.SetText(fmt.Sprintf(
			" User: [yellow]%s[white] | TCP:%s UDP:%s WS:%s | %s",
			userDisplay, tcpDot, udpDot, wsDot, msg,
		))
	})
}

func (s *AppState) AddNotification(notif string) {
	s.mu.Lock()
	entry := fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), notif)
	s.NotificationList = append([]string{entry}, s.NotificationList...)
	if len(s.NotificationList) > 50 {
		s.NotificationList = s.NotificationList[:50]
	}
	s.mu.Unlock()

	s.App.QueueUpdateDraw(func() {
		if s.NotifView != nil {
			var fullText string
			s.mu.RLock()
			for _, n := range s.NotificationList {
				fullText += n + "\n\n"
			}
			s.mu.RUnlock()
			s.NotifView.SetText(fullText)
		}
		s.SetStatus(fmt.Sprintf("[yellow]🔔 %s[white]", notif))
	})
}

