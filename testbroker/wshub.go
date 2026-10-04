package main

import (
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

type wsMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// WSHub manages WebSocket connections and broadcasts per user.
type WSHub struct {
	mu       sync.RWMutex
	clients  map[string]map[*websocket.Conn]struct{} // userID -> set of connections
	upgrader websocket.Upgrader
	cfg      *Config
}

// NewWSHub creates a new WebSocket hub.
func NewWSHub(cfg *Config) *WSHub {
	return &WSHub{
		clients: make(map[string]map[*websocket.Conn]struct{}),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins for the test broker
			},
		},
		cfg: cfg,
	}
}

// HandleWS upgrades the HTTP connection to a WebSocket.
func (h *WSHub) HandleWS(w http.ResponseWriter, r *http.Request) {
	apiKey := r.URL.Query().Get("api_key")
	accessToken := r.URL.Query().Get("access_token")

	userID, ok := h.cfg.UserByToken(apiKey, accessToken)
	if !ok {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WS upgrade failed: %v", err)
		return
	}

	h.register(userID, conn)

	// Keep reading until connection closes
	go func() {
		defer h.unregister(userID, conn)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				break
			}
		}
	}()
}

func (h *WSHub) register(userID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[userID] == nil {
		h.clients[userID] = make(map[*websocket.Conn]struct{})
	}
	h.clients[userID][conn] = struct{}{}
}

func (h *WSHub) unregister(userID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if conns, ok := h.clients[userID]; ok {
		delete(conns, conn)
		if len(conns) == 0 {
			delete(h.clients, userID)
		}
	}
	conn.Close()
}

// Broadcast sends an order update to all clients connected as the order's UserID.
func (h *WSHub) Broadcast(o *Order) {
	h.mu.RLock()
	conns, ok := h.clients[o.UserID]
	if !ok || len(conns) == 0 {
		h.mu.RUnlock()
		return
	}
	
	// Copy the connections to avoid holding the lock during write
	targets := make([]*websocket.Conn, 0, len(conns))
	for conn := range conns {
		targets = append(targets, conn)
	}
	h.mu.RUnlock()

	msg := wsMessage{
		Type: "order",
		Data: o,
	}

	for _, conn := range targets {
		err := conn.WriteJSON(msg)
		if err != nil {
			log.Printf("WS write error for user %s: %v", o.UserID, err)
			h.unregister(o.UserID, conn)
		}
	}
}
