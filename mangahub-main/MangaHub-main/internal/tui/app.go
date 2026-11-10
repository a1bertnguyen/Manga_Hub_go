package tui

import (
	"fmt"
	"os"

	"mangahub/internal/config"

	"github.com/rivo/tview"
)

func Run() error {
	cfg := config.Load()

	baseURL := fmt.Sprintf("http://localhost:%s", cfg.HTTPPort)
	tcpAddr := fmt.Sprintf("localhost:%s", cfg.TCPPort)
	udpAddr := fmt.Sprintf("localhost:%s", cfg.UDPPort)
	wsURL := fmt.Sprintf("ws://localhost:%s/ws", cfg.HTTPPort)

	// Allow environment overrides for remote/custom test setups
	if envAPI := os.Getenv("API_URL"); envAPI != "" {
		baseURL = envAPI
	}
	if envTCP := os.Getenv("TCP_ADDR"); envTCP != "" {
		tcpAddr = envTCP
	}
	if envUDP := os.Getenv("UDP_ADDR"); envUDP != "" {
		udpAddr = envUDP
	}
	if envWS := os.Getenv("WS_URL"); envWS != "" {
		wsURL = envWS
	}

	app := tview.NewApplication()
	pages := tview.NewPages()

	state := NewAppState(app, pages, baseURL, tcpAddr, udpAddr, wsURL)
	api := NewAPIClient(state)
	tcp := NewTCPWorker(state)
	udp := NewUDPWorker(state)
	chat := NewChatClient(state)

	controller := NewUIController(state, api, tcp, udp, chat)
	rootLayout := controller.SetupLayout()

	app.SetRoot(rootLayout, true).EnableMouse(true)
	return app.Run()
}

