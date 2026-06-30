package daemon

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

// WSEvent is a message sent to connected WebSocket clients.
type WSEvent struct {
	Type    string `json:"type"`
	Project string `json:"project,omitempty"` // project slug
	Data    any    `json:"data,omitempty"`
}

const (
	EventRequestCreated     = "request_created"
	EventRequestResolved    = "request_resolved"
	EventCredentialsChanged = "credentials_changed"
	EventNotesChanged       = "notes_changed"
)

// upgrader only accepts WebSocket handshakes from non-browser clients (the
// Swift app sends no Origin) or from a loopback browser/WKWebView origin — so a
// website the user visits cannot open the daemon's event stream. The Host check
// in localhostGuard applies to the upgrade request too.
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		return strings.HasPrefix(origin, "http://127.0.0.1:") ||
			strings.HasPrefix(origin, "http://localhost:") ||
			strings.HasPrefix(origin, "http://[::1]:")
	},
}

// WSHub manages WebSocket connections grouped by project slug.
type WSHub struct {
	mu      sync.RWMutex
	clients map[*wsClient]struct{}
}

type wsClient struct {
	conn    *websocket.Conn
	project string // empty = all projects
	send    chan []byte
}

func NewWSHub() *WSHub {
	return &WSHub{
		clients: make(map[*wsClient]struct{}),
	}
}

// Broadcast sends an event to all clients watching the given project (or all projects).
func (h *WSHub) Broadcast(event WSEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for c := range h.clients {
		if c.project == "" || c.project == event.Project {
			select {
			case c.send <- data:
			default:
				// client too slow, skip
			}
		}
	}
}

// HasClients returns true if any WebSocket client is watching the given project.
func (h *WSHub) HasClients(project string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		if c.project == "" || c.project == project {
			return true
		}
	}
	return false
}

func (h *WSHub) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws upgrade: %v", err)
		return
	}

	project := r.URL.Query().Get("project")
	client := &wsClient{
		conn:    conn,
		project: project,
		send:    make(chan []byte, 16),
	}

	h.mu.Lock()
	h.clients[client] = struct{}{}
	h.mu.Unlock()

	// Writer goroutine
	go func() {
		defer conn.Close()
		for msg := range client.send {
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				break
			}
		}
	}()

	// Reader goroutine — just drain reads to detect disconnect
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}

	h.mu.Lock()
	delete(h.clients, client)
	h.mu.Unlock()
	close(client.send)
}
