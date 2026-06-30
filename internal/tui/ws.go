package tui

import (
	"encoding/json"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gorilla/websocket"
)

// wsEventMsg represents a WebSocket event from the daemon.
type wsEventMsg struct {
	Type    string `json:"type"`
	Project string `json:"project,omitempty"`
}

// wsListener connects to the daemon WebSocket and forwards events to the
// Bubble Tea program. It reconnects automatically on failure.
func wsListener(baseURL string, p *tea.Program) {
	wsURL := strings.Replace(baseURL, "http://", "ws://", 1)
	wsURL = strings.Replace(wsURL, "https://", "wss://", 1)
	wsURL += "/api/ws"

	for {
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}

		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				conn.Close()
				break
			}

			var event wsEventMsg
			if err := json.Unmarshal(message, &event); err != nil {
				continue
			}
			p.Send(event)
		}

		time.Sleep(2 * time.Second)
	}
}
